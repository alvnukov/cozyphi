package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// meterInterval is how often a recording reports its level and elapsed time.
const meterInterval = 100 * time.Millisecond

// State is what the session is doing right now.
type State int

// The session states, in the order one recording walks through them.
const (
	StateIdle State = iota
	StateRecording
	StateTranscribing
)

// String names the state for status output.
func (s State) String() string {
	switch s {
	case StateRecording:
		return "recording"
	case StateTranscribing:
		return "transcribing"
	case StateIdle:
		return "idle"
	default:
		return "idle"
	}
}

// EventKind distinguishes the three things a session reports.
type EventKind int

// The event kinds.
const (
	// EventState reports a transition or a meter tick.
	EventState EventKind = iota
	// EventResult carries a finished transcript.
	EventResult
	// EventError carries a one-sentence failure.
	EventError
	// EventNotice carries something worth saying that is not a failure, such
	// as the max_seconds auto-stop.
	EventNotice
)

// Event is one report from the session. Gen identifies the recording it
// belongs to, so the UI can drop a result that arrived after a cancel.
type Event struct {
	Kind     EventKind
	Gen      int
	State    State
	Elapsed  time.Duration
	Level    float64
	Text     string
	Language string
	Hint     string
}

// stopReason says why a recording ended.
type stopReason int

const (
	stopByUser stopReason = iota
	stopByCancel
	stopByLimit
	stopByCapture
)

// Options builds a Session. Capture and Transcriber are seams: leave them nil
// and the session derives them from Resolved when a recording starts.
type Options struct {
	Config      Config
	Resolved    Resolved
	WAVPath     string
	Capture     Capture
	Transcriber Transcriber
}

// Session owns one voice recording at a time and reports through a callback.
// It knows nothing about the TUI: the callback runs on a background goroutine
// and the caller is responsible for hopping to the UI thread.
type Session struct {
	mu   sync.Mutex
	opts Options
	emit func(Event)

	gen   int
	state State
	rec   *recording

	// lastWAV is the audio of the most recent recording, kept until a
	// transcription of it succeeds so /voice retry has something to send.
	lastWAV []byte
}

// recording is one in-flight capture.
type recording struct {
	gen      int
	stream   Stream
	cancel   context.CancelFunc
	stop     chan stopReason
	stopOnce sync.Once
}

func (r *recording) signal(reason stopReason) {
	r.stopOnce.Do(func() { r.stop <- reason })
}

// NewSession builds a session. emit may be nil in tests that only drive state.
func NewSession(opts Options, emit func(Event)) *Session {
	if emit == nil {
		emit = func(Event) {}
	}
	return &Session{opts: opts, emit: emit}
}

// State reports what the session is doing. Safe from any goroutine.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Config exposes the decoded configuration for status output.
func (s *Session) Config() Config { return s.opts.Config }

// Resolved exposes what the loader found on this machine.
func (s *Session) Resolved() Resolved { return s.opts.Resolved }

// HasRecording reports whether /voice retry has audio to resend.
func (s *Session) HasRecording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lastWAV) > 0
}

// Toggle is the keybinding's action: start when idle, stop when recording,
// and say so while a transcription is still running.
func (s *Session) Toggle(ctx context.Context) {
	switch s.State() {
	case StateIdle:
		s.Start(ctx)
	case StateRecording:
		s.Stop()
	case StateTranscribing:
		s.emit(Event{Kind: EventNotice, Gen: s.currentGen(), Text: "still transcribing…"})
	}
}

func (s *Session) currentGen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// Start begins a recording. A disabled or unconfigured session reports the
// reason and stays idle.
func (s *Session) Start(ctx context.Context) {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return
	}
	if !s.opts.Config.Enabled {
		gen := s.gen
		s.mu.Unlock()
		s.fail(gen, errors.New("voice input is off"), "set voice.enabled: true in config.yaml")
		return
	}
	if hint := s.opts.Resolved.Hint(); hint != "" {
		gen := s.gen
		s.mu.Unlock()
		s.fail(gen, errors.New(hint), "")
		return
	}
	capture, err := s.captureLocked()
	if err != nil {
		gen := s.gen
		s.mu.Unlock()
		s.fail(gen, err, "")
		return
	}
	s.gen++
	gen := s.gen
	s.mu.Unlock()

	lifetime, cancel := context.WithCancel(ctx)
	stream, err := capture.Start(lifetime, s.opts.Config.Capture.Device)
	if err != nil {
		cancel()
		s.fail(gen, err, "")
		return
	}

	rec := &recording{gen: gen, stream: stream, cancel: cancel, stop: make(chan stopReason, 1)}
	s.mu.Lock()
	s.state = StateRecording
	s.rec = rec
	s.mu.Unlock()

	s.emit(Event{Kind: EventState, Gen: gen, State: StateRecording})
	// run watches the capture through lifetime but finishes the recording under
	// ctx: stopping the capture must not cancel the transcription that follows.
	go s.run(ctx, lifetime, rec)
}

// Stop ends the recording and transcribes what was heard.
func (s *Session) Stop() {
	s.mu.Lock()
	rec := s.rec
	s.mu.Unlock()
	if rec != nil {
		rec.signal(stopByUser)
	}
}

// Cancel throws the recording away without transcribing, and abandons a
// running transcription. It is silent by design: Esc says "never mind".
func (s *Session) Cancel() {
	s.mu.Lock()
	rec := s.rec
	gen := s.gen
	// Bumping the generation orphans a transcription that is already in
	// flight: its result and its errors are dropped on arrival.
	s.gen++
	if s.state == StateTranscribing {
		s.state = StateIdle
	}
	s.mu.Unlock()
	if rec != nil {
		rec.signal(stopByCancel)
		return
	}
	s.emit(Event{Kind: EventState, Gen: gen, State: StateIdle})
}

// Close kills any capture in flight. The editor calls it on shutdown so no
// ffmpeg outlives the TUI.
func (s *Session) Close() {
	s.mu.Lock()
	rec := s.rec
	s.rec = nil
	s.state = StateIdle
	s.gen++
	s.mu.Unlock()
	if rec != nil {
		rec.cancel()
		_, _ = rec.stream.Stop()
	}
}

// Retry transcribes the audio of the last recording again, which is what a
// failed transcription leaves behind.
func (s *Session) Retry(ctx context.Context) {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return
	}
	wav := s.lastWAV
	if len(wav) == 0 {
		gen := s.gen
		s.mu.Unlock()
		s.fail(gen, errors.New("no recording to retry"), "record something with the voice key first")
		return
	}
	s.gen++
	gen := s.gen
	s.state = StateTranscribing
	s.mu.Unlock()

	s.emit(Event{Kind: EventState, Gen: gen, State: StateTranscribing})
	go s.transcribe(ctx, gen, wav)
}

// run watches one recording: it ticks the meter, enforces max_seconds, and
// notices a capture that died on its own.
func (s *Session) run(ctx, lifetime context.Context, rec *recording) {
	ticker := time.NewTicker(meterInterval)
	defer ticker.Stop()
	limit := time.NewTimer(time.Duration(s.opts.Config.MaxSeconds) * time.Second)
	defer limit.Stop()

	for {
		select {
		case reason := <-rec.stop:
			s.finish(ctx, rec, reason)
			return
		case <-lifetime.Done():
			s.finish(ctx, rec, stopByCancel)
			return
		case <-rec.stream.Done():
			s.finish(ctx, rec, stopByCapture)
			return
		case <-limit.C:
			s.finish(ctx, rec, stopByLimit)
			return
		case <-ticker.C:
			s.emit(Event{
				Kind:    EventState,
				Gen:     rec.gen,
				State:   StateRecording,
				Elapsed: rec.stream.Duration(),
				Level:   rec.stream.Level(),
			})
		}
	}
}

// finish closes the capture and decides what happens to the audio.
func (s *Session) finish(ctx context.Context, rec *recording, reason stopReason) {
	elapsed := rec.stream.Duration()
	samples, err := rec.stream.Stop()
	rec.cancel()

	s.mu.Lock()
	current := s.rec == rec
	if current {
		s.rec = nil
		s.state = StateIdle
	}
	gen := s.gen
	s.mu.Unlock()

	if !current || gen != rec.gen || reason == stopByCancel {
		s.emit(Event{Kind: EventState, Gen: rec.gen, State: StateIdle})
		return
	}
	if err != nil {
		s.fail(rec.gen, err, "")
		return
	}
	if IsSilent(samples) {
		s.remember(EncodeWAV(samples, SampleRate))
		s.fail(rec.gen, errors.New("heard only silence"), "check the input device with /voice devices")
		return
	}

	wav := EncodeWAV(samples, SampleRate)
	s.remember(wav)
	if reason == stopByLimit {
		s.emit(Event{
			Kind: EventNotice,
			Gen:  rec.gen,
			Text: fmt.Sprintf("recording stopped at %s (voice.max_seconds)", formatElapsed(elapsed)),
		})
	}

	s.mu.Lock()
	s.state = StateTranscribing
	s.mu.Unlock()
	s.emit(Event{Kind: EventState, Gen: rec.gen, State: StateTranscribing})
	s.transcribe(ctx, rec.gen, wav)
}

// transcribe sends the audio to the configured backend and reports the text.
func (s *Session) transcribe(ctx context.Context, gen int, wav []byte) {
	defer func() {
		s.mu.Lock()
		if s.gen == gen && s.state == StateTranscribing {
			s.state = StateIdle
		}
		s.mu.Unlock()
	}()

	tr, err := s.ensureTranscriber()
	if err != nil {
		s.fail(gen, err, "")
		return
	}
	res, err := tr.Transcribe(ctx, Request{
		WAV:      wav,
		Language: s.opts.Config.Language,
		Prompt:   s.opts.Config.Hint(),
	})
	if s.stale(gen) {
		return
	}
	if err != nil {
		s.fail(gen, err, "")
		return
	}
	text := strings.TrimSpace(res.Text)
	if text == "" {
		s.fail(gen, errors.New("transcription returned no text"),
			"speak closer to the microphone; /voice retry keeps the recording")
		return
	}
	s.forget()
	s.emit(Event{Kind: EventState, Gen: gen, State: StateIdle})
	s.emit(Event{Kind: EventResult, Gen: gen, Text: text, Language: res.Language})
}

// stale reports whether a newer recording has superseded gen.
func (s *Session) stale(gen int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen != gen
}

// fail reports a failure and leaves the session idle.
func (s *Session) fail(gen int, err error, hint string) {
	s.emit(Event{Kind: EventState, Gen: gen, State: StateIdle})
	s.emit(Event{Kind: EventError, Gen: gen, Text: err.Error(), Hint: hint})
}

// remember keeps the audio for /voice retry, on disk as well when a path is
// configured. A write failure is not worth interrupting the user for: the
// in-memory copy still serves the retry.
func (s *Session) remember(wav []byte) {
	s.mu.Lock()
	s.lastWAV = wav
	path := s.opts.WAVPath
	s.mu.Unlock()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, wav, 0o600)
}

// forget drops the recording once it has been transcribed successfully, so
// the microphone leaves nothing behind on disk.
func (s *Session) forget() {
	s.mu.Lock()
	s.lastWAV = nil
	path := s.opts.WAVPath
	s.mu.Unlock()
	if path != "" {
		_ = os.Remove(path)
	}
}

// captureLocked builds the capture backend once and caches it. The caller
// holds s.mu.
func (s *Session) captureLocked() (Capture, error) {
	if s.opts.Capture != nil {
		return s.opts.Capture, nil
	}
	argv := s.opts.Resolved.Capture.Argv
	if len(argv) == 0 {
		return nil, errors.New("no capture command found — install ffmpeg or set voice.capture.command")
	}
	s.opts.Capture = NewCommandCapture(argv, s.opts.Config.MaxSeconds)
	return s.opts.Capture, nil
}

// ensureTranscriber builds the speech backend once and caches it.
func (s *Session) ensureTranscriber() (Transcriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opts.Transcriber != nil {
		return s.opts.Transcriber, nil
	}
	timeout := time.Duration(s.opts.Config.STT.TimeoutSeconds) * time.Second
	var (
		tr  Transcriber
		err error
	)
	switch s.opts.Resolved.STT.Backend {
	case BackendCommand:
		tr, err = NewCommandTranscriber(s.opts.Resolved.STT.Command, s.opts.Resolved.STT.ModelPath, timeout)
	case BackendHTTP:
		tr, err = NewHTTPTranscriber(
			s.opts.Config.STT.BaseURL,
			s.opts.Config.STT.Model,
			s.opts.Config.STT.apiKey,
			timeout,
		)
	case BackendAuto:
		err = errors.New(s.opts.Resolved.STT.Hint)
	default:
		err = errors.New(s.opts.Resolved.STT.Hint)
	}
	if err != nil {
		return nil, err
	}
	s.opts.Transcriber = tr
	return tr, nil
}

// Status is the one-line answer to /voice status.
func (s *Session) Status() string {
	cfg := s.opts.Config
	res := s.opts.Resolved
	if !cfg.Enabled {
		return "voice: off (set voice.enabled: true)"
	}
	if hint := res.Hint(); hint != "" {
		return "voice: not ready — " + hint
	}
	capture := "none"
	if len(res.Capture.Argv) > 0 {
		capture = filepath.Base(res.Capture.Argv[0])
	}
	stt := string(res.STT.Backend)
	if res.STT.Backend == BackendCommand {
		if argv, err := splitArgs(res.STT.Command); err == nil && len(argv) > 0 {
			stt = filepath.Base(argv[0])
		}
		if res.STT.ModelPath != "" {
			stt += " (" + filepath.Base(res.STT.ModelPath) + ")"
		}
	}
	if res.STT.Backend == BackendHTTP {
		stt += " (" + cfg.STT.BaseURL + ")"
	}
	return fmt.Sprintf("voice: %s — capture %s on %q, transcriber %s, language %s, max %ds",
		s.State(), capture, cfg.Capture.Device, stt, cfg.Language, cfg.MaxSeconds)
}

// formatElapsed renders a duration as m:ss, the way the meter shows it.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
