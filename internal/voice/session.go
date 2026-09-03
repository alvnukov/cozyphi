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

const (
	// meterInterval is how often the loop drains the stream, feeds the
	// segmenter and reports the meter level.
	meterInterval = 100 * time.Millisecond
	// segmentQueue bounds the segments waiting for the transcriber. When it
	// fills the oldest is dropped: the microphone must never block on a slow
	// backend, and stale audio is the least valuable thing to keep.
	segmentQueue = 8
	// pauseGrace is how long a paused mode keeps the capture process alive,
	// reading and discarding audio, so push-to-talk and a quick resume do not
	// pay the device-open cost.
	pauseGrace = 30 * time.Second
	// previousTail is how much of the previous transcript rides along as
	// context, in runes, so a sentence split across segments keeps its thread.
	previousTail = 200
	// cmdBuffer bounds the command channel. Commands come from keystrokes, so
	// a handful of slots is more than a human can fill.
	cmdBuffer = 8
)

// State is what the session is doing right now.
type State int

// The session states. The mode is on in every state but StateIdle.
const (
	// StateIdle means the mode is off; the microphone is not held.
	StateIdle State = iota
	// StateListening means the capture runs and the segmenter is cutting it.
	StateListening
	// StatePaused means the mode is on but audio is discarded.
	StatePaused
	// StateFinishing means the mode is ending and the queue is draining.
	StateFinishing
)

// String names the state for status output.
func (s State) String() string {
	switch s {
	case StateListening:
		return "listening"
	case StatePaused:
		return "paused"
	case StateFinishing:
		return "finishing"
	case StateIdle:
		return "idle"
	default:
		return "idle"
	}
}

// EventKind distinguishes the four things a session reports.
type EventKind int

// The event kinds.
const (
	// EventState reports a transition or a meter tick.
	EventState EventKind = iota
	// EventResult carries one finished segment's transcript.
	EventResult
	// EventError carries a one-sentence failure.
	EventError
	// EventNotice carries something worth saying that is not a failure, such
	// as the silence auto-pause.
	EventNotice
)

// Event is one report from the session. Gen identifies the mode it belongs to,
// so the UI can drop a result that arrived after the mode ended, and Seq
// identifies the segment inside that mode. Every event carries Level, Pending
// and State, so the UI can render the whole indicator from the latest event.
type Event struct {
	Kind     EventKind
	Gen      int
	Seq      int
	State    State
	Level    float64
	Pending  int
	Starting bool
	Text     string
	Language string
	Hint     string
}

// Options builds a Session. Capture and Transcriber are seams: leave them nil
// and the session derives them from Resolved when the mode starts.
type Options struct {
	Config      Config
	Resolved    Resolved
	WAVPath     string
	Capture     Capture
	Transcriber Transcriber
	// HoldKeys says whether the terminal delivers key releases, which decides
	// whether Status may promise push-to-talk.
	HoldKeys bool
}

// Session owns the voice dialog mode and reports through a callback. It knows
// nothing about the TUI: the callback runs on background goroutines and the
// caller is responsible for hopping to the UI thread.
//
// Three goroutines meet here. The caller's goroutine only ever takes s.mu
// briefly and posts a command; the mode's loop owns the capture stream and the
// segmenter and is the sole sender on the queue; the worker owns the
// transcriber and drains the queue in order. Shared state — the state itself,
// the meter level, the pending count, the last transcript and the failed
// segment — lives on the Session behind s.mu so any of them can build a
// complete Event.
type Session struct {
	mu   sync.Mutex
	opts Options
	emit func(Event)

	gen      int
	seq      int
	state    State
	level    float64
	starting bool
	pending  int
	mode     *mode

	// lastText is the previous successful transcript, whose tail primes the
	// next segment's prompt.
	lastText string
	// failedWAV is the audio of the last segment that failed, kept until a
	// retry succeeds or the mode ends so /voice retry has something to send.
	failedWAV []byte
	failedSeq int

	// grace is pauseGrace, overridable in tests.
	grace time.Duration
}

// mode is one run of the dialog mode: a generation, a lifetime and the two
// goroutines that serve it.
type mode struct {
	gen        int
	ctx        context.Context
	cancel     context.CancelFunc
	cmds       chan modeCmd
	queue      chan segment
	loopDone   chan struct{}
	workerDone chan struct{}
}

// modeCmd is a request from the UI goroutine to the mode's loop.
type modeCmd struct {
	kind cmdKind
	wav  []byte
	seq  int
}

type cmdKind int

const (
	cmdPause cmdKind = iota
	cmdResume
	cmdFlush
	cmdEnd
	cmdRetry
)

// segment is one closed utterance on its way to the transcriber.
type segment struct {
	seq int
	wav []byte
}

// NewSession builds a session. emit may be nil in tests that only drive state.
func NewSession(opts Options, emit func(Event)) *Session {
	if emit == nil {
		emit = func(Event) {}
	}
	return &Session{opts: opts, emit: emit, grace: pauseGrace}
}

// State reports what the session is doing. Safe from any goroutine.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Pending reports how many segments are queued or being transcribed.
func (s *Session) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

// HasFailed reports whether /voice retry has a failed segment to resend.
func (s *Session) HasFailed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.failedWAV) > 0
}

// Config exposes the decoded configuration for status output.
func (s *Session) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opts.Config
}

// Resolved exposes what the loader found on this machine.
func (s *Session) Resolved() Resolved {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opts.Resolved
}

// Reconfigure swaps the configuration of an idle session, so a model installed
// while cozyphi runs takes effect without a restart. The cached transcriber is
// dropped: the next segment builds one from the new resolution.
func (s *Session) Reconfigure(cfg Config, resolved Resolved) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateIdle {
		return errors.New("voice is still running")
	}
	s.opts.Config = cfg
	s.opts.Resolved = resolved
	s.opts.Transcriber = nil
	return nil
}

// Start enters the mode and opens the microphone. A disabled or unconfigured
// session reports the reason and stays idle.
func (s *Session) Start(ctx context.Context) {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return
	}
	gen := s.gen
	if !s.opts.Config.Enabled {
		s.mu.Unlock()
		s.fail(gen, errors.New("voice input is off"), "set voice.enabled: true in config.yaml")
		return
	}
	if hint := s.opts.Resolved.Hint(); hint != "" {
		s.mu.Unlock()
		s.fail(gen, errors.New(hint), "")
		return
	}
	capture, err := s.captureLocked()
	if err != nil {
		s.mu.Unlock()
		s.fail(gen, err, "")
		return
	}
	device := s.opts.Config.Capture.Device
	s.mu.Unlock()

	lifetime, cancel := context.WithCancel(ctx)
	stream, err := capture.Start(lifetime, device)
	if err != nil {
		cancel()
		s.fail(gen, err, "")
		return
	}

	m := &mode{
		ctx:        lifetime,
		cancel:     cancel,
		cmds:       make(chan modeCmd, cmdBuffer),
		queue:      make(chan segment, segmentQueue),
		loopDone:   make(chan struct{}),
		workerDone: make(chan struct{}),
	}
	s.mu.Lock()
	s.gen++
	m.gen = s.gen
	s.seq = 0
	s.pending = 0
	s.level = 0
	// The device takes a moment to open, and the hint row says so until the
	// first audio arrives — the same promise a restart after a pause makes.
	s.starting = true
	s.lastText = ""
	s.state = StateListening
	s.mode = m
	s.mu.Unlock()

	go s.worker(m)
	go s.loop(m, stream)
	s.emitState(m.gen)
}

// Pause stops feeding the segmenter; the capture process stays alive for the
// grace period so a resume is instant.
func (s *Session) Pause() { s.post(modeCmd{kind: cmdPause}) }

// Resume goes back to listening, restarting the capture when the grace period
// already closed it. The context is accepted for symmetry with Start; the
// restart runs under the mode's own lifetime, which Close and Discard cancel.
func (s *Session) Resume(_ context.Context) { s.post(modeCmd{kind: cmdResume}) }

// Flush closes the open segment now, so Enter and Ctrl+G do not lose the words
// spoken up to the keystroke.
func (s *Session) Flush() { s.post(modeCmd{kind: cmdFlush}) }

// End leaves the mode keeping what was said: the open segment is closed, the
// queue drains, then the session reports StateIdle.
func (s *Session) End() { s.post(modeCmd{kind: cmdEnd}) }

// Retry re-queues the last failed segment. The editor gates it on HasFailed,
// so a session with nothing to retry simply does nothing here.
func (s *Session) Retry(_ context.Context) {
	s.mu.Lock()
	wav, seq := s.failedWAV, s.failedSeq
	s.mu.Unlock()
	if len(wav) == 0 {
		return
	}
	s.post(modeCmd{kind: cmdRetry, wav: wav, seq: seq})
}

// post hands a command to the mode's loop. It never blocks: a loop that has
// already finished simply has nothing left to do.
func (s *Session) post(cmd modeCmd) {
	s.mu.Lock()
	m := s.mode
	s.mu.Unlock()
	if m == nil {
		return
	}
	select {
	case m.cmds <- cmd:
	default:
	}
}

// Discard leaves the mode and throws everything away: the open segment, the
// queue and any transcription in flight. It reports StateIdle at once, because
// Esc must feel immediate; the goroutines wind down behind it.
func (s *Session) Discard() {
	if m := s.teardown(); m != nil {
		m.cancel()
	}
	s.emitState(s.currentGen())
}

// Close kills the capture and drops the queue without reporting anything. The
// editor calls it on every quit path, so no ffmpeg outlives the TUI.
func (s *Session) Close() {
	m := s.teardown()
	if m == nil {
		return
	}
	m.cancel()
	<-m.loopDone
}

// teardown ends the mode under the lock and returns it, or nil when the mode
// was already off. Bumping the generation orphans everything in flight: those
// events are dropped on arrival.
func (s *Session) teardown() *mode {
	s.mu.Lock()
	m := s.mode
	s.mode = nil
	s.gen++
	s.state = StateIdle
	s.pending = 0
	s.level = 0
	s.starting = false
	s.lastText = ""
	s.failedWAV = nil
	s.mu.Unlock()
	s.forget()
	return m
}

func (s *Session) currentGen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// loop owns the capture stream and the segmenter for one mode. Everything that
// touches audio happens here, so the segmenter needs no lock of its own.
func (s *Session) loop(m *mode, stream Stream) {
	defer close(m.loopDone)

	seg := newSegmenter(s.opts.Config)
	ticker := time.NewTicker(meterInterval)
	defer ticker.Stop()
	grace := time.NewTimer(s.grace)
	grace.Stop()
	defer grace.Stop()

	l := &loopState{sess: s, mode: m, seg: seg, stream: stream, grace: grace}
	defer l.closeStream()

	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd := <-m.cmds:
			if l.command(cmd) {
				return
			}
		case <-grace.C:
			l.graceExpired()
		case <-doneChan(l.stream):
			l.captureDied()
		case <-ticker.C:
			l.tick()
		}
	}
}

// loopState is the loop's own state, split out so each transition reads as one
// short method instead of a branch inside a select.
type loopState struct {
	sess   *Session
	mode   *mode
	seg    *segmenter
	stream Stream
	grace  *time.Timer
	paused bool
}

// doneChan returns the stream's completion channel, or nil when there is no
// stream, which parks that arm of the select forever.
func doneChan(stream Stream) <-chan struct{} {
	if stream == nil {
		return nil
	}
	return stream.Done()
}

// tick drains the stream once, feeds the segmenter and reports the meter.
func (l *loopState) tick() {
	if l.stream == nil {
		return
	}
	chunk := l.stream.Drain()
	if l.paused {
		// Audio heard while paused is dropped on purpose, and the segmenter
		// stays reset so nothing from before the pause joins the next
		// segment. Nothing changed, so nothing is reported either.
		return
	}
	if len(chunk) > 0 && l.sess.clearStarting() {
		l.sess.emitState(l.mode.gen)
	}
	for _, s := range l.seg.Push(chunk) {
		l.sess.enqueue(l.mode, s)
	}
	l.sess.setLevel(l.stream.Level())
	if limit := l.sess.autoPause(); limit > 0 && l.seg.Silence() >= limit {
		l.sess.emitEvent(Event{
			Kind: EventNotice,
			Gen:  l.mode.gen,
			Text: fmt.Sprintf("paused after %s of silence — Space resumes", formatElapsed(limit)),
		})
		l.pause()
		return
	}
	l.sess.emitState(l.mode.gen)
}

// command applies one UI command and reports whether the loop is finished.
func (l *loopState) command(cmd modeCmd) bool {
	switch cmd.kind {
	case cmdPause:
		l.pause()
	case cmdResume:
		l.resume()
	case cmdFlush:
		l.flush()
	case cmdRetry:
		l.sess.send(l.mode, segment{seq: cmd.seq, wav: cmd.wav})
	case cmdEnd:
		l.end()
		return true
	}
	return false
}

// pause discards what the segmenter holds and arms the grace timer.
func (l *loopState) pause() {
	if l.paused {
		return
	}
	l.paused = true
	l.seg.Reset()
	l.sess.setState(StatePaused, false)
	l.sess.setLevel(0)
	if l.stream != nil {
		l.grace.Reset(l.sess.grace)
	}
	l.sess.emitState(l.mode.gen)
}

// resume goes back to listening, reopening the device when the grace period
// already closed it. A device that will not open leaves the mode paused with a
// toast, which is the only honest outcome: there is nothing to listen to.
func (l *loopState) resume() {
	if !l.paused {
		return
	}
	l.grace.Stop()
	l.paused = false
	l.seg.Reset()
	if l.stream == nil {
		l.sess.setState(StateListening, true)
		l.sess.emitState(l.mode.gen)
		stream, err := l.sess.startStream()
		if err != nil {
			l.paused = true
			l.sess.setState(StatePaused, false)
			l.sess.emitState(l.mode.gen)
			l.sess.emitEvent(Event{
				Kind: EventError,
				Gen:  l.mode.gen,
				Text: err.Error(),
				Hint: "Space retries the microphone",
			})
			return
		}
		l.stream = stream
		return
	}
	// The device never closed, so there is no start to wait for. Drop what it
	// heard while paused rather than replaying it.
	l.stream.Drain()
	l.sess.setState(StateListening, false)
	l.sess.emitState(l.mode.gen)
}

// flush closes the open segment, if it holds any speech.
func (l *loopState) flush() {
	if l.paused || l.stream == nil {
		return
	}
	for _, s := range l.seg.Push(l.stream.Drain()) {
		l.sess.enqueue(l.mode, s)
	}
	if s := l.seg.Flush(); len(s) > 0 {
		l.sess.enqueue(l.mode, s)
	}
}

// end closes the microphone, lets the worker drain and reports StateIdle.
func (l *loopState) end() {
	l.flush()
	l.closeStream()
	l.sess.setState(StateFinishing, false)
	l.sess.setLevel(0)
	l.sess.emitState(l.mode.gen)

	close(l.mode.queue)
	select {
	case <-l.mode.workerDone:
	case <-l.mode.ctx.Done():
		// Discard or Close overtook the drain; they already reported Idle.
		return
	}
	if m := l.sess.teardown(); m != nil {
		m.cancel()
	}
	l.sess.emitState(l.sess.currentGen())
}

// graceExpired releases the device after a long pause.
func (l *loopState) graceExpired() {
	if !l.paused {
		return
	}
	l.closeStream()
}

// captureDied handles a capture process that ended on its own. While listening
// that is a failure worth a toast; the mode falls back to paused, where a
// resume will try the device again.
func (l *loopState) captureDied() {
	stream := l.stream
	l.stream = nil
	if stream == nil {
		return
	}
	_, err := stream.Stop()
	if l.paused {
		return
	}
	l.paused = true
	l.sess.setState(StatePaused, false)
	l.sess.setLevel(0)
	l.sess.emitState(l.mode.gen)
	if err == nil {
		err = errors.New("capture stopped unexpectedly")
	}
	l.sess.emitEvent(Event{Kind: EventError, Gen: l.mode.gen, Text: err.Error(), Hint: "Space retries the microphone"})
}

// closeStream stops the capture if one is open.
func (l *loopState) closeStream() {
	if l.stream == nil {
		return
	}
	stream := l.stream
	l.stream = nil
	_, _ = stream.Stop()
}

// startStream opens the capture device again after the grace period.
func (s *Session) startStream() (Stream, error) {
	s.mu.Lock()
	capture, err := s.captureLocked()
	m := s.mode
	device := s.opts.Config.Capture.Device
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("voice dialog is off")
	}
	return capture.Start(m.ctx, device)
}

// enqueue encodes a closed segment and hands it to the worker.
func (s *Session) enqueue(m *mode, samples []int16) {
	s.mu.Lock()
	seq := s.seq
	s.seq++
	s.mu.Unlock()
	s.send(m, segment{seq: seq, wav: EncodeWAV(samples, SampleRate)})
}

// send queues a segment without ever blocking the capture loop. A full queue
// means the transcriber is behind; the oldest segment goes, because the newest
// words are the ones the user is waiting for.
func (s *Session) send(m *mode, seg segment) {
	s.addPending(1)
	for range segmentQueue + 1 {
		select {
		case m.queue <- seg:
			s.emitState(m.gen)
			return
		default:
		}
		select {
		case <-m.queue:
			s.addPending(-1)
			s.emitEvent(Event{
				Kind: EventNotice,
				Gen:  m.gen,
				Text: "transcriber is behind, dropped one segment",
			})
		default:
		}
	}
	// Unreachable in practice: the loop is the only sender, so freeing a slot
	// guarantees the next send fits. Keep the count honest anyway.
	s.addPending(-1)
}

// worker transcribes segments one at a time, in order, for one mode.
func (s *Session) worker(m *mode) {
	defer close(m.workerDone)
	for seg := range m.queue {
		if m.ctx.Err() != nil {
			s.addPending(-1)
			continue
		}
		s.transcribe(m, seg)
	}
}

// transcribe sends one segment to the backend and reports what came back.
func (s *Session) transcribe(m *mode, seg segment) {
	defer func() {
		s.addPending(-1)
		s.emitState(m.gen)
	}()

	tr, err := s.ensureTranscriber()
	if err != nil {
		s.failSegment(m, seg, err)
		return
	}
	res, err := tr.Transcribe(m.ctx, Request{
		WAV:      seg.wav,
		Language: s.opts.Config.Language,
		Prompt:   s.prompt(),
	})
	if m.ctx.Err() != nil {
		return
	}
	if err != nil {
		s.failSegment(m, seg, err)
		return
	}
	text := strings.TrimSpace(res.Text)
	s.succeed(seg.seq, text)
	if text == "" {
		// A segment the backend heard nothing in is not an error: RMS says
		// someone spoke, the model disagreed. Saying so once per pause would
		// be noise, so it passes quietly.
		return
	}
	s.emitEvent(Event{
		Kind:     EventResult,
		Gen:      m.gen,
		Seq:      seg.seq,
		Text:     text,
		Language: res.Language,
	})
}

// prompt is the vocabulary hint followed by the tail of the previous result.
func (s *Session) prompt() string {
	hint := s.opts.Config.Hint()
	s.mu.Lock()
	tail := s.lastText
	s.mu.Unlock()
	if runes := []rune(tail); len(runes) > previousTail {
		tail = string(runes[len(runes)-previousTail:])
	}
	switch {
	case hint == "":
		return tail
	case tail == "":
		return hint
	default:
		return hint + " " + tail
	}
}

// succeed records a transcribed segment and forgets the retry copy when the
// segment that failed is the one that just came back.
func (s *Session) succeed(seq int, text string) {
	s.mu.Lock()
	if text != "" {
		s.lastText = text
	}
	drop := len(s.failedWAV) > 0 && s.failedSeq == seq
	if drop {
		s.failedWAV = nil
	}
	s.mu.Unlock()
	if drop {
		s.forget()
	}
}

// failSegment reports a segment that could not be transcribed and keeps its
// audio for /voice retry.
func (s *Session) failSegment(m *mode, seg segment, err error) {
	s.remember(seg)
	s.emitEvent(Event{Kind: EventError, Gen: m.gen, Seq: seg.seq, Text: err.Error()})
}

// emitState reports the current state, level and pending count for gen.
func (s *Session) emitState(gen int) {
	s.emitEvent(Event{Kind: EventState, Gen: gen})
}

// emitEvent fills in the shared fields and delivers the event, unless a newer
// mode has already superseded it.
func (s *Session) emitEvent(ev Event) {
	s.mu.Lock()
	if ev.Gen != s.gen {
		s.mu.Unlock()
		return
	}
	ev.State = s.state
	ev.Level = s.level
	ev.Pending = s.pending
	ev.Starting = s.starting
	emit := s.emit
	s.mu.Unlock()
	emit(ev)
}

// fail reports a failure that never reached a segment, leaving the mode off.
func (s *Session) fail(gen int, err error, hint string) {
	s.emitEvent(Event{Kind: EventState, Gen: gen})
	s.emitEvent(Event{Kind: EventError, Gen: gen, Text: err.Error(), Hint: hint})
}

func (s *Session) setState(state State, starting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.starting = starting
}

func (s *Session) setLevel(level float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.level = level
}

// clearStarting drops the "starting" flag on the first audio and reports
// whether it was set, so the caller knows the UI needs an update.
func (s *Session) clearStarting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.starting {
		return false
	}
	s.starting = false
	return true
}

func (s *Session) addPending(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending += delta
	if s.pending < 0 {
		s.pending = 0
	}
}

// autoPause is the configured continuous silence that pauses the mode.
func (s *Session) autoPause() time.Duration {
	return time.Duration(s.opts.Config.AutoPauseSeconds) * time.Second
}

// remember keeps a failed segment for /voice retry, on disk as well when a
// path is configured. A write failure is not worth interrupting the user for:
// the in-memory copy still serves the retry.
func (s *Session) remember(seg segment) {
	s.mu.Lock()
	s.failedWAV = seg.wav
	s.failedSeq = seg.seq
	path := s.opts.WAVPath
	s.mu.Unlock()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, seg.wav, 0o600)
}

// forget removes the retry copy from disk, so the microphone leaves nothing
// behind once the audio is no longer needed.
func (s *Session) forget() {
	s.mu.Lock()
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
	cfg := s.Config()
	res := s.Resolved()
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
	tail := fmt.Sprintf("capture %s on %q, transcriber %s, language %s, segment %ds",
		capture, cfg.Capture.Device, stt, cfg.Language, cfg.MaxSeconds)

	state, pending := s.State(), s.Pending()
	if state == StateIdle {
		return "voice: idle — " + tail
	}
	queued := ""
	if pending > 0 {
		queued = fmt.Sprintf(" (%d queued)", pending)
	}
	hold := "no"
	if s.opts.HoldKeys {
		hold = "yes"
	}
	return fmt.Sprintf("voice: dialog %s%s, hold keys %s — %s", state, queued, hold, tail)
}

// formatElapsed renders a duration as m:ss, the way a timer shows it.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
