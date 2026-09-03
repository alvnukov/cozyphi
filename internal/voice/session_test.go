package voice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const waitDeadline = 3 * time.Second

// fakeStream is a recording that is already finished: it hands back a fixed
// set of samples and remembers that it was stopped.
type fakeStream struct {
	mu      sync.Mutex
	samples []int16
	stopErr error
	stops   int
	done    chan struct{}
}

func newFakeStream(samples []int16) *fakeStream {
	return &fakeStream{samples: samples, done: make(chan struct{})}
}

func (*fakeStream) Level() float64          { return 0.5 }
func (f *fakeStream) Samples() []int16      { return f.samples }
func (*fakeStream) Duration() time.Duration { return 2 * time.Second }
func (f *fakeStream) Done() <-chan struct{} { return f.done }
func (f *fakeStream) die()                  { close(f.done) }

func (f *fakeStream) Stop() ([]int16, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return f.samples, f.stopErr
}

func (f *fakeStream) stopped() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stops
}

// stubCapture hands out one prepared stream.
type stubCapture struct {
	mu      sync.Mutex
	stream  *fakeStream
	err     error
	devices []string
}

func (f *stubCapture) Start(_ context.Context, device string) (Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices = append(f.devices, device)
	if f.err != nil {
		return nil, f.err
	}
	return f.stream, nil
}

// fakeTranscriber records what it was asked to transcribe and can be held open
// to model a slow backend.
type fakeTranscriber struct {
	mu      sync.Mutex
	reqs    []Request
	results []Result
	errs    []error
	entered chan struct{}
	release chan struct{}
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, req Request) (Result, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	n := len(f.reqs) - 1
	var (
		res Result
		err error
	)
	if n < len(f.results) {
		res = f.results[n]
	} else if len(f.results) > 0 {
		res = f.results[len(f.results)-1]
	}
	if n < len(f.errs) {
		err = f.errs[n]
	}
	entered, release := f.entered, f.release
	f.mu.Unlock()

	if entered != nil {
		close(entered)
		f.mu.Lock()
		f.entered = nil
		f.mu.Unlock()
	}
	if release != nil {
		select {
		case <-release:
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

func ofKind(k EventKind) func(Event) bool {
	return func(e Event) bool { return e.Kind == k }
}

// fakeSampleCount is a quarter second at 16 kHz: long enough for the silence
// check to have something to measure, short enough to stay cheap.
const fakeSampleCount = 4000

func loudSamples() []int16 {
	out := make([]int16, fakeSampleCount)
	for i := range out {
		out[i] = 9000
	}
	return out
}

func quietSamples() []int16 {
	out := make([]int16, fakeSampleCount)
	for i := range out {
		out[i] = 10
	}
	return out
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
	return NewSession(opts, log.emit), log
}

func waitIdle(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		if s.State() == StateIdle {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("session stayed %s", s.State())
}

func TestSessionTranscribesWhatItRecorded(t *testing.T) {
	capture := &stubCapture{stream: newFakeStream(loudSamples())}
	stt := &fakeTranscriber{results: []Result{{Text: "hello there", Language: "en"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = capture
		o.Transcriber = stt
	})

	s.Start(t.Context())
	log.waitFor(t, "recording", func(e Event) bool { return e.Kind == EventState && e.State == StateRecording })
	s.Stop()

	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "hello there", got.Text)
	assert.Equal(t, "en", got.Language)
	assert.Equal(t, []string{DefaultDevice}, capture.devices)

	reqs := stt.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, DefaultLanguage, reqs[0].Language)
	assert.Equal(t, "cozyphi, worktree, goreleaser, xui", reqs[0].Prompt)
	assert.Equal(t, "RIFF", string(reqs[0].WAV[0:4]))

	waitIdle(t, s)
	assert.False(t, s.HasRecording(), "a successful transcription drops the audio")
}

func TestSessionReportsSilenceAndKeepsTheAudio(t *testing.T) {
	stt := &fakeTranscriber{}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: newFakeStream(quietSamples())}
		o.Transcriber = stt
	})

	s.Start(t.Context())
	s.Stop()

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "heard only silence", got.Text)
	assert.Equal(t, "check the input device with /voice devices", got.Hint)
	assert.Empty(t, stt.requests(), "silence never reaches the transcriber")
	assert.True(t, s.HasRecording(), "the audio stays for /voice retry")
}

func TestSessionCancelIsSilent(t *testing.T) {
	stream := newFakeStream(loudSamples())
	stt := &fakeTranscriber{}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: stream}
		o.Transcriber = stt
	})

	s.Start(t.Context())
	log.waitFor(t, "recording", func(e Event) bool { return e.Kind == EventState && e.State == StateRecording })
	s.Cancel()

	log.waitFor(t, "idle", func(e Event) bool { return e.Kind == EventState && e.State == StateIdle })
	waitIdle(t, s)
	assert.Positive(t, stream.stopped(), "the capture is stopped on cancel")
	assert.Empty(t, stt.requests(), "a canceled recording is never transcribed")
	_, hasResult := log.find(ofKind(EventResult))
	assert.False(t, hasResult)
	_, hasError := log.find(ofKind(EventError))
	assert.False(t, hasError, "Esc says nothing")
}

func TestSessionCancelOrphansAnInFlightTranscription(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	stt := &fakeTranscriber{
		results: []Result{{Text: "too late"}},
		entered: entered,
		release: release,
	}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = stt
	})

	s.Start(t.Context())
	s.Stop()
	select {
	case <-entered:
	case <-time.After(waitDeadline):
		t.Fatal("the transcriber was never called")
	}

	s.Cancel()
	close(release)

	time.Sleep(50 * time.Millisecond)
	_, hasResult := log.find(ofKind(EventResult))
	assert.False(t, hasResult, "a result that arrives after a cancel is dropped")
	assert.Equal(t, StateIdle, s.State())
}

func TestSessionRetryResendsTheSameAudio(t *testing.T) {
	stt := &fakeTranscriber{
		results: []Result{{}, {Text: "second time lucky"}},
		errs:    []error{errors.New("transcription failed (HTTP 500) — /voice retry keeps the recording")},
	}
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = stt
	})

	s.Start(t.Context())
	s.Stop()
	log.waitFor(t, "error", ofKind(EventError))
	waitIdle(t, s)
	require.True(t, s.HasRecording())

	s.Retry(t.Context())
	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "second time lucky", got.Text)

	reqs := stt.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, reqs[0].WAV, reqs[1].WAV, "retry resends the recording it kept")
}

func TestSessionRetryWithoutARecordingExplainsItself(t *testing.T) {
	s, log := newTestSession(t, nil)

	s.Retry(t.Context())
	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "no recording to retry", got.Text)
	assert.Equal(t, "record something with the voice key first", got.Hint)
}

func TestSessionStopsAtMaxSeconds(t *testing.T) {
	stt := &fakeTranscriber{results: []Result{{Text: "cut short"}}}
	s, log := newTestSession(t, func(o *Options) {
		o.Config.MaxSeconds = 0 // the limit timer fires at once
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = stt
	})

	s.Start(t.Context())

	notice := log.waitFor(t, "notice", ofKind(EventNotice))
	assert.Contains(t, notice.Text, "(voice.max_seconds)")
	got := log.waitFor(t, "result", ofKind(EventResult))
	assert.Equal(t, "cut short", got.Text, "the audio recorded before the limit is still transcribed")
}

func TestSessionReportsACaptureThatDies(t *testing.T) {
	stream := newFakeStream(nil)
	stream.stopErr = errors.New(`capture produced no audio from "default" — try /voice devices`)
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: stream}
		o.Transcriber = &fakeTranscriber{}
	})

	s.Start(t.Context())
	stream.die()

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Contains(t, got.Text, "capture produced no audio")
}

func TestSessionReportsAnEmptyTranscript(t *testing.T) {
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = &fakeTranscriber{results: []Result{{Text: "   "}}}
	})

	s.Start(t.Context())
	s.Stop()

	got := log.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "transcription returned no text", got.Text)
	assert.Contains(t, got.Hint, "/voice retry keeps the recording")
}

func TestSessionCloseKillsTheCapture(t *testing.T) {
	stream := newFakeStream(loudSamples())
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: stream}
		o.Transcriber = &fakeTranscriber{}
	})

	s.Start(t.Context())
	log.waitFor(t, "recording", func(e Event) bool { return e.Kind == EventState && e.State == StateRecording })
	s.Close()

	assert.Positive(t, stream.stopped(), "shutdown stops the capture process")
	assert.Equal(t, StateIdle, s.State())
}

func TestSessionRefusesToStartWhenDisabledOrUnconfigured(t *testing.T) {
	off, offLog := newTestSession(t, func(o *Options) { o.Config.Enabled = false })
	off.Start(t.Context())
	got := offLog.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "voice input is off", got.Text)
	assert.Equal(t, "set voice.enabled: true in config.yaml", got.Hint)

	unready, unreadyLog := newTestSession(t, func(o *Options) {
		o.Resolved = Resolved{Capture: ResolvedCapture{Hint: "no capture command found — install ffmpeg"}}
	})
	unready.Start(t.Context())
	got = unreadyLog.waitFor(t, "error", ofKind(EventError))
	assert.Equal(t, "no capture command found — install ffmpeg", got.Text)
}

func TestSessionTogglesBetweenRecordingAndTranscribing(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	s, log := newTestSession(t, func(o *Options) {
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = &fakeTranscriber{results: []Result{{Text: "done"}}, entered: entered, release: release}
	})

	assert.Equal(t, StateIdle, s.State())
	s.Toggle(t.Context())
	log.waitFor(t, "recording", func(e Event) bool { return e.Kind == EventState && e.State == StateRecording })

	s.Toggle(t.Context()) // stops
	select {
	case <-entered:
	case <-time.After(waitDeadline):
		t.Fatal("the transcriber was never called")
	}

	s.Toggle(t.Context()) // busy
	notice := log.waitFor(t, "notice", ofKind(EventNotice))
	assert.Equal(t, "still transcribing…", notice.Text)
}

func TestSessionKeepsTheLastRecordingPrivateOnDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "voice")
	path := filepath.Join(dir, "last.wav")
	stt := &fakeTranscriber{
		results: []Result{{}, {Text: "recovered"}},
		errs:    []error{errors.New("transcription failed")},
	}
	s, log := newTestSession(t, func(o *Options) {
		o.WAVPath = path
		o.Capture = &stubCapture{stream: newFakeStream(loudSamples())}
		o.Transcriber = stt
	})

	s.Start(t.Context())
	s.Stop()
	log.waitFor(t, "error", ofKind(EventError))
	waitIdle(t, s)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the recording is readable only by its owner")
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	s.Retry(t.Context())
	log.waitFor(t, "result", ofKind(EventResult))
	waitIdle(t, s)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "a successful transcription removes the recording")
}

func TestSessionStatusDescribesTheSetup(t *testing.T) {
	ready, _ := newTestSession(t, nil)
	assert.Equal(t,
		`voice: idle — capture ffmpeg on "default", transcriber whisper-cli (ggml-base.bin), language auto, max 300s`,
		ready.Status())

	off, _ := newTestSession(t, func(o *Options) { o.Config.Enabled = false })
	assert.Equal(t, "voice: off (set voice.enabled: true)", off.Status())

	unready, _ := newTestSession(t, func(o *Options) {
		o.Resolved = Resolved{Capture: ResolvedCapture{Hint: "install ffmpeg"}}
	})
	assert.Equal(t, "voice: not ready — install ffmpeg", unready.Status())
}

func TestStateStringNamesEveryState(t *testing.T) {
	assert.Equal(t, "idle", StateIdle.String())
	assert.Equal(t, "recording", StateRecording.String())
	assert.Equal(t, "transcribing", StateTranscribing.String())
}

func TestFormatElapsedRendersMinutesAndSeconds(t *testing.T) {
	assert.Equal(t, "0:00", formatElapsed(-time.Second))
	assert.Equal(t, "0:07", formatElapsed(7*time.Second))
	assert.Equal(t, "5:00", formatElapsed(5*time.Minute))
	assert.Equal(t, "12:34", formatElapsed(12*time.Minute+34*time.Second))
}
