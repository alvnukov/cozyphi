package voice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/redact"
)

const (
	// captureStartTimeout is how long we wait for the first PCM byte before
	// declaring the device dead. ffmpeg opens a working device well inside it.
	captureStartTimeout = 3 * time.Second
	// captureChunk is the read size: 4096 bytes is 128 ms at 16 kHz mono,
	// small enough for a lively meter and large enough to stay cheap.
	captureChunk = 4096
	// levelSmoothing weights the previous meter level against the new one, so
	// the bar follows the voice instead of flickering with every chunk.
	levelSmoothing = 0.6
	// captureStderrLimit bounds the stderr tail kept for the error message.
	captureStderrLimit = 8 << 10
)

// Capture starts one microphone recording. The implementation runs an external
// command, because release builds are CGO_ENABLED=0 and cannot link a portable
// audio library.
type Capture interface {
	Start(ctx context.Context, device string) (Stream, error)
}

// Stream is a recording in progress. Every method is safe to call from the UI
// goroutine while the reader goroutine appends samples.
type Stream interface {
	// Level is the smoothed 0..1 meter level.
	Level() float64
	// Samples is a copy of everything captured so far.
	Samples() []int16
	// Duration is how long the recording has been running.
	Duration() time.Duration
	// Done closes when capture ended on its own — the process died, or no
	// audio ever arrived. Stop then reports why.
	Done() <-chan struct{}
	// Stop ends the recording, kills the process and returns what was heard.
	Stop() ([]int16, error)
}

// CommandCapture runs a command that writes s16le mono PCM to stdout.
type CommandCapture struct {
	argv       []string
	maxSamples int
	goos       string
}

// NewCommandCapture builds a Capture from a resolved argv. maxSeconds bounds
// the sample buffer so a forgotten recording cannot grow without limit.
func NewCommandCapture(argv []string, maxSeconds int) *CommandCapture {
	if maxSeconds <= 0 {
		maxSeconds = DefaultMaxSeconds
	}
	return &CommandCapture{
		argv: append([]string(nil), argv...),
		// Two seconds of slack over the session's own auto-stop: the buffer is
		// a memory guard, not the timer.
		maxSamples: (maxSeconds + 2) * SampleRate,
		goos:       runtime.GOOS,
	}
}

// Start launches the capture command with {device} expanded.
func (c *CommandCapture) Start(ctx context.Context, device string) (Stream, error) {
	if len(c.argv) == 0 {
		return nil, errors.New("no capture command configured")
	}
	argv := expandArgs(c.argv, map[string]string{"device": device})
	lifetime, cancel := context.WithCancel(ctx)
	p, err := proc.Start(lifetime, proc.Spec{Argv: argv}, captureStderrLimit)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cannot start %s: %s", argv[0], redact.Redact(err.Error()))
	}
	s := &commandStream{
		proc:       p,
		cancel:     cancel,
		device:     device,
		goos:       c.goos,
		maxSamples: c.maxSamples,
		started:    time.Now(),
		done:       make(chan struct{}),
	}
	go s.read()
	return s, nil
}

// commandStream reads PCM from the capture process.
type commandStream struct {
	proc       *proc.Process
	cancel     context.CancelFunc
	device     string
	goos       string
	maxSamples int
	started    time.Time
	done       chan struct{}
	doneOnce   sync.Once

	mu      sync.Mutex
	samples []int16
	level   float64
	heard   bool
	readErr error
}

func (s *commandStream) read() {
	defer s.finish()
	timer := time.AfterFunc(captureStartTimeout, func() {
		if !s.heardAnything() {
			s.cancel()
		}
	})
	defer timer.Stop()

	buf := make([]byte, captureChunk)
	stdout := s.proc.Stdout()
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			s.append(DecodePCM(buf[:n]))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.setReadErr(err)
			}
			return
		}
	}
}

func (s *commandStream) append(chunk []int16) {
	level := LevelFromRMS(RMS(chunk))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heard = true
	if room := s.maxSamples - len(s.samples); room > 0 {
		if len(chunk) > room {
			chunk = chunk[:room]
		}
		s.samples = append(s.samples, chunk...)
	}
	s.level = s.level*levelSmoothing + level*(1-levelSmoothing)
}

func (s *commandStream) setReadErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr == nil {
		s.readErr = err
	}
}

func (s *commandStream) heardAnything() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heard
}

func (s *commandStream) finish() {
	s.doneOnce.Do(func() { close(s.done) })
}

// Level returns the smoothed meter level.
func (s *commandStream) Level() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.level
}

// Samples copies what has been captured so far.
func (s *commandStream) Samples() []int16 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int16(nil), s.samples...)
}

// Duration reports how long the recording has run.
func (s *commandStream) Duration() time.Duration { return time.Since(s.started) }

// Done closes when the capture process ended by itself.
func (s *commandStream) Done() <-chan struct{} { return s.done }

// Stop kills the capture process, waits for the reader, and returns the audio.
// A recording that produced no bytes at all is an error naming the device.
func (s *commandStream) Stop() ([]int16, error) {
	s.cancel()
	_ = s.proc.Close(0)
	<-s.done

	samples := s.Samples()
	s.mu.Lock()
	heard, readErr := s.heard, s.readErr
	s.mu.Unlock()

	if heard {
		return samples, nil
	}
	return nil, s.noAudioError(readErr)
}

// noAudioError explains an empty recording, naming the device and the next
// thing to try. The stderr tail is redacted before it is shown.
func (s *commandStream) noAudioError(readErr error) error {
	var b strings.Builder
	fmt.Fprintf(&b, "capture produced no audio from %q — try /voice devices", s.device)
	if s.goos == "darwin" {
		b.WriteString(" and allow microphone access for your terminal")
	}
	if tail := strings.TrimSpace(s.proc.StderrTail()); tail != "" {
		fmt.Fprintf(&b, " (%s)", redact.Redact(firstLine(tail)))
	} else if readErr != nil {
		fmt.Fprintf(&b, " (%s)", redact.Redact(readErr.Error()))
	}
	return errors.New(b.String())
}

func firstLine(s string) string {
	if head, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(head)
	}
	return s
}
