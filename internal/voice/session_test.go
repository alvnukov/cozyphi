package voice

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const waitDeadline = 3 * time.Second

// scriptStream is a microphone the test speaks into: feed hands it audio, the
// session drains it on its next tick. Nothing here runs a process, so the whole
// suite is hardware-free.
type scriptStream struct {
	mu      sync.Mutex
	pending []int16
	stops   int
	stopErr error
	done    chan struct{}
	dieOnce sync.Once
}

func newScriptStream() *scriptStream {
	return &scriptStream{done: make(chan struct{})}
}

// feed queues audio for the next Drain.
func (s *scriptStream) feed(chunk []int16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, chunk...)
}

func (s *scriptStream) Drain() []int16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pending
	s.pending = nil
	return out
}

func (*scriptStream) Level() float64          { return 0.5 }
func (*scriptStream) Samples() []int16        { return nil }
func (*scriptStream) Duration() time.Duration { return time.Second }
func (s *scriptStream) Done() <-chan struct{} { return s.done }

// die models a capture process that ended on its own.
func (s *scriptStream) die() { s.dieOnce.Do(func() { close(s.done) }) }

func (s *scriptStream) Stop() ([]int16, error) {
	s.die()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	return nil, s.stopErr
}

func (s *scriptStream) stopped() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stops
}

// stubCapture hands out prepared streams, one per Start, repeating the last.
type stubCapture struct {
	mu      sync.Mutex
	streams []*scriptStream
	starts  int
	err     error
	devices []string
}

func newStubCapture(streams ...*scriptStream) *stubCapture {
	if len(streams) == 0 {
		streams = []*scriptStream{newScriptStream()}
	}
	return &stubCapture{streams: streams}
}

func (f *stubCapture) Start(_ context.Context, device string) (Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices = append(f.devices, device)
	if f.err != nil {
		return nil, f.err
	}
	stream := f.streams[min(f.starts, len(f.streams)-1)]
	f.starts++
	return stream, nil
}

func (f *stubCapture) started() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts
}

// fakeTranscriber answers from a script and can be held open to model a slow
// backend, which is how the queue tests fill the channel.
type fakeTranscriber struct {
	mu      sync.Mutex
	reqs    []Request
	results []Result
	errs    []error
	gate    chan struct{}
	entered chan struct{}
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, req Request) (Result, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	n := len(f.reqs) - 1
	var (
		res Result
		err error
	)
	switch {
	case n < len(f.results):
		res = f.results[n]
	case len(f.results) > 0:
		res = f.results[len(f.results)-1]
	}
	if n < len(f.errs) {
		err = f.errs[n]
	}
	gate, entered := f.gate, f.entered
	f.mu.Unlock()

	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	return res, err
}

func (f *fakeTranscriber) requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.reqs...)
}

// eventLog collects what the session reported.
type eventLog struct {
	mu   sync.Mutex
	list []Event
}

func (l *eventLog) emit(e Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, e)
}

func (l *eventLog) all() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Event(nil), l.list...)
}

func (l *eventLog) of(kind EventKind) []Event {
	var out []Event
	for _, e := range l.all() {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func (l *eventLog) find(pred func(Event) bool) (Event, bool) {
	for _, e := range l.all() {
		if pred(e) {
			return e, true
		}
	}
	return Event{}, false
}

func (l *eventLog) waitFor(t *testing.T, what string, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if e, ok := l.find(pred); ok {
			return e
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no %s event; saw %+v", what, l.all())
	return Event{}
}

func (l *eventLog) waitCount(t *testing.T, what string, kind EventKind, n int) []Event {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if got := l.of(kind); len(got) >= n {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("wanted %d %s events; saw %+v", n, what, l.all())
	return nil
}

func ofKind(k EventKind) func(Event) bool {
	return func(e Event) bool { return e.Kind == k }
}

func inState(s State) func(Event) bool {
	return func(e Event) bool { return e.Kind == EventState && e.State == s }
}

// utterance is a word followed by enough silence to close the segment.
func utterance() []int16 {
	return join(tone(300*time.Millisecond), hush(1200*time.Millisecond))
}

func readyResolved() Resolved {
	return Resolved{
		Capture: ResolvedCapture{Argv: []string{"ffmpeg", "-i", "x"}},
		STT: ResolvedSTT{
			Backend:   BackendCommand,
			Command:   "whisper-cli -f {file}",
			ModelPath: "/models/ggml-base.bin",
		},
	}
}

func newTestSession(t *testing.T, mutate func(o *Options)) (*Session, *eventLog) {
	t.Helper()
	opts := Options{Config: Defaults(), Resolved: readyResolved()}
	if mutate != nil {
		mutate(&opts)
	}
	log := &eventLog{}
	s := NewSession(opts, log.emit)
	t.Cleanup(s.Close)
	return s, log
}

// startListening starts the mode and waits until the loop is running.
func startListening(t *testing.T, s *Session, log *eventLog) {
	t.Helper()
	s.Start(t.Context())
	log.waitFor(t, "listening", inState(StateListening))
}

func waitState(t *testing.T, s *Session, want State) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if s.State() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("session stayed %s, wanted %s", s.State(), want)
}

func waitPending(t *testing.T, s *Session, want int) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if s.Pending() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("pending stayed %d, wanted %d", s.Pending(), want)
}

func waitTrue(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSessionTranscribesEveryUtteranceInOrder(t *testing.T) {
	stream := newScriptStream()
	capture := newStubCapture(stream)
	stt := &fakeTranscriber{results: []Result{
		{Text: "first thought", Language: "en"},
		{Text: "second thought", Language: "en"},
	}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = capture
		o.Transcriber = stt
	})

	startListening(t, s, log)
	stream.feed(join(utterance(), utterance()))

	results := log.waitCount(t, "result", EventResult, 2)
	assert.Equal(t, "first thought", results[0].Text)
	assert.Equal(t, "en", results[0].Language)
	assert.Equal(t, 0, results[0].Seq)
	assert.Equal(t, "second thought", results[1].Text)
	assert.Equal(t, 1, results[1].Seq)
	assert.Equal(t, []string{DefaultDevice}, capture.devices)

	reqs := stt.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, DefaultLanguage, reqs[0].Language)
	assert.Equal(t, "cozyphi, worktree, goreleaser, xui", reqs[0].Prompt)
	// The second segment carries the first transcript, so a sentence split
	// across a pause keeps its thread.
	assert.Equal(t, "cozyphi, worktree, goreleaser, xui first thought", reqs[1].Prompt)
	assert.Equal(t, "RIFF", string(reqs[0].WAV[0:4]))
}

func TestSessionPromptKeepsOnlyTheTailOfThePreviousResult(t *testing.T) {
	stream := newScriptStream()
	long := strings.Repeat("a", previousTail+50)
	stt := &fakeTranscriber{results: []Result{{Text: long}, {Text: "next"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
		o.Config.Hints = HintsOff
	})

	startListening(t, s, log)
	stream.feed(utterance())
	log.waitCount(t, "result", EventResult, 1)
	stream.feed(utterance())
	log.waitCount(t, "result", EventResult, 2)

	reqs := stt.requests()
	require.Len(t, reqs, 2)
	assert.Empty(t, reqs[0].Prompt)
	assert.Equal(t, strings.Repeat("a", previousTail), reqs[1].Prompt)
}

func TestSessionCountsPendingSegments(t *testing.T) {
	stream := newScriptStream()
	gate := make(chan struct{})
	stt := &fakeTranscriber{results: []Result{{Text: "one"}, {Text: "two"}}, gate: gate}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})

	assert.Equal(t, 0, s.Pending())
	startListening(t, s, log)
	stream.feed(join(utterance(), utterance()))
	waitPending(t, s, 2)

	pending := log.waitFor(t, "two pending", func(e Event) bool { return e.Pending == 2 })
	assert.Equal(t, StateListening, pending.State)

	close(gate)
	waitPending(t, s, 0)
}

func TestSessionKeepsGoingAfterAFailedSegment(t *testing.T) {
	stream := newScriptStream()
	wav := t.TempDir() + "/last.wav"
	stt := &fakeTranscriber{
		results: []Result{{}, {Text: "still here"}},
		errs:    []error{errors.New("transcriber said no")},
	}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
		o.WAVPath = wav
	})

	startListening(t, s, log)
	stream.feed(join(utterance(), utterance()))

	failed := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "transcriber said no", failed.Text)
	assert.Equal(t, 0, failed.Seq)

	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "still here", got.Text)
	assert.Equal(t, 1, got.Seq)

	assert.True(t, s.HasFailed())
	info, err := os.Stat(wav)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSessionRetryResendsTheFailedSegment(t *testing.T) {
	stream := newScriptStream()
	wav := t.TempDir() + "/last.wav"
	stt := &fakeTranscriber{
		results: []Result{{}, {Text: "second time lucky"}},
		errs:    []error{errors.New("transcriber said no")},
	}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
		o.WAVPath = wav
	})

	startListening(t, s, log)
	stream.feed(utterance())
	log.waitFor(t, "error", ofKind(EventError))
	require.True(t, s.HasFailed())

	s.Retry(t.Context())

	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "second time lucky", got.Text)
	// The retry keeps the segment's own number, so the text lands where the
	// failed one would have.
	assert.Equal(t, 0, got.Seq)
	waitTrue(t, "retry cleared", func() bool { return !s.HasFailed() })
	_, err := os.Stat(wav)
	assert.True(t, os.IsNotExist(err))
}

func TestSessionRetryWithoutAFailureDoesNothing(t *testing.T) {
	stream := newScriptStream()
	stt := &fakeTranscriber{results: []Result{{Text: "fine"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})

	startListening(t, s, log)
	s.Retry(t.Context())

	assert.False(t, s.HasFailed())
	assert.Empty(t, stt.requests())
}

func TestSessionStartPromisesTheOpeningDevice(t *testing.T) {
	stream := newScriptStream()
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = &fakeTranscriber{}
	})

	s.Start(t.Context())

	// ffmpeg needs a moment to open the device, and the first open says so
	// exactly as a restart after the grace period does.
	starting := log.waitFor(t, "starting", func(e Event) bool { return e.Kind == EventState && e.Starting })
	assert.Equal(t, StateListening, starting.State)

	stream.feed(utterance())
	log.waitFor(t, "started", func(e Event) bool {
		return e.Kind == EventState && e.State == StateListening && !e.Starting
	})
}

func TestSessionPauseDiscardsAudio(t *testing.T) {
	stream := newScriptStream()
	stt := &fakeTranscriber{results: []Result{{Text: "heard it"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})

	startListening(t, s, log)
	s.Pause()
	waitState(t, s, StatePaused)

	stream.feed(join(utterance(), utterance()))
	// Three meter intervals is plenty for the loop to have drained the audio
	// it must throw away.
	time.Sleep(3 * meterInterval)
	assert.Empty(t, stt.requests())
	assert.Equal(t, 0, s.Pending())

	s.Resume(t.Context())
	waitState(t, s, StateListening)
	stream.feed(utterance())

	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "heard it", got.Text)
	assert.Len(t, stt.requests(), 1)
}

func TestSessionReleasesTheDeviceAfterTheGracePeriod(t *testing.T) {
	first, second := newScriptStream(), newScriptStream()
	capture := newStubCapture(first, second)
	stt := &fakeTranscriber{results: []Result{{Text: "back again"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = capture
		o.Transcriber = stt
	})
	s.grace = 20 * time.Millisecond

	startListening(t, s, log)
	s.Pause()
	waitTrue(t, "device released", func() bool { return first.stopped() > 0 })

	s.Resume(t.Context())

	starting := log.waitFor(t, "starting", func(e Event) bool { return e.Kind == EventState && e.Starting })
	assert.Equal(t, StateListening, starting.State)
	waitTrue(t, "device reopened", func() bool { return capture.started() == 2 })

	second.feed(utterance())
	log.waitFor(t, "result", ofKind(EventResult))
	// The indicator stops promising a start once audio actually arrives.
	log.waitFor(t, "started", func(e Event) bool {
		return e.Kind == EventState && e.State == StateListening && !e.Starting
	})
}

func TestSessionResumeReportsADeviceThatWillNotOpen(t *testing.T) {
	stream := newScriptStream()
	capture := newStubCapture(stream)
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = capture
		o.Transcriber = &fakeTranscriber{}
	})
	s.grace = 20 * time.Millisecond

	startListening(t, s, log)
	s.Pause()
	waitTrue(t, "device released", func() bool { return stream.stopped() > 0 })

	capture.mu.Lock()
	capture.err = errors.New("no such device")
	capture.mu.Unlock()
	s.Resume(t.Context())

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "no such device", got.Text)
	assert.Equal(t, "Space retries the microphone", got.Hint,
		"the mode stays paused, so the toast has to name the key that tries again")
	assert.Equal(t, StatePaused, s.State())
}

func TestSessionEndDrainsTheQueueThenGoesIdle(t *testing.T) {
	stream := newScriptStream()
	stt := &fakeTranscriber{results: []Result{{Text: "last words"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})

	startListening(t, s, log)
	// Speech with no trailing pause: only the flush inside End saves it.
	stream.feed(tone(400 * time.Millisecond))
	time.Sleep(2 * meterInterval)
	s.End()

	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "last words", got.Text)
	log.waitFor(t, "finishing", inState(StateFinishing))
	log.waitFor(t, "idle", inState(StateIdle))
	waitState(t, s, StateIdle)
	assert.Positive(t, stream.stopped())
}

func TestSessionDiscardDropsEverything(t *testing.T) {
	stream := newScriptStream()
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	stt := &fakeTranscriber{results: []Result{{Text: "never seen"}}, gate: gate, entered: entered}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})
	defer close(gate)

	startListening(t, s, log)
	stream.feed(utterance())
	select {
	case <-entered:
	case <-time.After(waitDeadline):
		t.Fatal("transcriber never started")
	}

	s.Discard()

	assert.Equal(t, StateIdle, s.State())
	assert.Equal(t, 0, s.Pending())
	last := log.all()
	require.NotEmpty(t, last)
	assert.Equal(t, StateIdle, last[len(last)-1].State)
	assert.Empty(t, log.of(EventResult))
	waitTrue(t, "capture stopped", func() bool { return stream.stopped() > 0 })
}

func TestSessionPausesItselfAfterLongSilence(t *testing.T) {
	stream := newScriptStream()
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = &fakeTranscriber{}
		o.Config.AutoPauseSeconds = 1
	})

	startListening(t, s, log)
	stream.feed(hush(1500 * time.Millisecond))

	got := log.waitFor(t, "notice", ofKind(EventNotice))
	assert.Equal(t, "paused after 0:01 of silence — Space resumes", got.Text)
	waitState(t, s, StatePaused)
}

func TestSessionDropsTheOldestSegmentWhenTheQueueIsFull(t *testing.T) {
	stream := newScriptStream()
	gate := make(chan struct{})
	stt := &fakeTranscriber{results: []Result{{Text: "slow"}}, gate: gate}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
	})
	defer close(gate)

	startListening(t, s, log)
	var speech []int16
	for range segmentQueue + 3 {
		speech = append(speech, utterance()...)
	}
	stream.feed(speech)

	got := log.waitFor(t, "notice", ofKind(EventNotice))
	assert.Equal(t, "transcriber is behind, dropped one segment", got.Text)
	// The queue is bounded, so the count never grows past what it can hold
	// plus the one being transcribed.
	assert.LessOrEqual(t, s.Pending(), segmentQueue+1)
}

func TestSessionReportsCaptureThatDies(t *testing.T) {
	stream := newScriptStream()
	stream.stopErr = errors.New("capture produced no audio from \"default\"")
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = &fakeTranscriber{}
	})

	startListening(t, s, log)
	stream.die()

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "capture produced no audio from \"default\"", got.Text)
	assert.Equal(t, "Space retries the microphone", got.Hint)
	waitState(t, s, StatePaused)
}

func TestSessionCloseKillsTheCapture(t *testing.T) {
	stream := newScriptStream()
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = &fakeTranscriber{}
	})

	startListening(t, s, log)
	s.Close()

	assert.Equal(t, StateIdle, s.State())
	assert.Positive(t, stream.stopped())
}

func TestSessionRefusesToStartWhenDisabled(t *testing.T) {
	s, log := newTestSession(t, func(o *Options) { o.Config.Enabled = false })

	s.Start(t.Context())

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "voice input is off", got.Text)
	assert.Equal(t, "set voice.enabled: true in config.yaml", got.Hint)
	assert.Equal(t, StateIdle, s.State())
}

func TestSessionRefusesToStartWhenUnresolved(t *testing.T) {
	s, log := newTestSession(t, func(o *Options) {
		o.Resolved = Resolved{STT: ResolvedSTT{Backend: BackendAuto, Hint: "install whisper-cli"}}
	})

	s.Start(t.Context())

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Contains(t, got.Text, "whisper-cli")
	assert.Equal(t, StateIdle, s.State())
}

func TestSessionStatusDescribesTheMode(t *testing.T) {
	stream := newScriptStream()
	gate := make(chan struct{})
	stt := &fakeTranscriber{results: []Result{{Text: "queued"}}, gate: gate}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = stt
		o.HoldKeys = func() bool { return true }
	})
	defer close(gate)

	assert.Equal(
		t,
		`voice: idle — capture ffmpeg on "default", transcriber whisper-cli (ggml-base.bin), language auto, segment 30s`,
		s.Status(),
	)

	startListening(t, s, log)
	stream.feed(join(utterance(), utterance()))
	waitPending(t, s, 2)

	assert.Equal(t,
		`voice: dialog listening (2 queued), hold keys yes — `+
			`capture ffmpeg on "default", transcriber whisper-cli (ggml-base.bin), language auto, segment 30s`,
		s.Status())
}

func TestSessionStatusSaysWhenHoldingIsUnavailable(t *testing.T) {
	stream := newScriptStream()
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = newStubCapture(stream)
		o.Transcriber = &fakeTranscriber{}
	})

	startListening(t, s, log)

	assert.Contains(t, s.Status(), "hold keys no")
}

func TestSessionStatusWithoutConfiguration(t *testing.T) {
	off, _ := newTestSession(t, func(o *Options) { o.Config.Enabled = false })
	assert.Equal(t, "voice: off (set voice.enabled: true)", off.Status())

	unready, _ := newTestSession(t, func(o *Options) {
		o.Resolved = Resolved{STT: ResolvedSTT{Backend: BackendAuto, Hint: "install whisper-cli"}}
	})
	assert.Equal(t, "voice: not ready — install whisper-cli", unready.Status())
}

func TestFormatElapsed(t *testing.T) {
	assert.Equal(t, "0:00", formatElapsed(0))
	assert.Equal(t, "0:05", formatElapsed(5*time.Second))
	assert.Equal(t, "5:00", formatElapsed(5*time.Minute))
	assert.Equal(t, "1:01", formatElapsed(61*time.Second))
}
