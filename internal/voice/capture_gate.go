package voice

import (
	"context"
	"errors"
	"sync"
)

// CaptureGate admits one recording across the captures wrapped by it. Share one
// gate across all retained Views to enforce process-wide microphone ownership.
// Switching Views does not transfer audio or release admission: the originating
// View must stop its recording first. The zero value is ready to use.
type CaptureGate struct {
	mu       sync.Mutex
	occupied bool
}

// NewCaptureGate returns an idle microphone admission gate.
func NewCaptureGate() *CaptureGate { return &CaptureGate{} }

// Wrap preserves a View's capture configuration while sharing admission. Neither
// a closed Done channel nor cancellation releases a successful stream's slot;
// Stop must finish cleaning up the actual recording first.
func (g *CaptureGate) Wrap(capture Capture) Capture {
	return &gatedCapture{gate: g, capture: capture}
}

// Busy reports whether the one recording this process admits is held, by this
// session or by another one. It is a snapshot of a moment: by the time a
// caller reads it the recording may have stopped, which is as much as any
// answer about a microphone is ever worth.
//
// It observes and returns. It admits nothing, releases nothing and opens no
// device. A nil gate is idle, the way a View wired without one is.
func (g *CaptureGate) Busy() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.occupied
}

func (g *CaptureGate) release() {
	g.mu.Lock()
	g.occupied = false
	g.mu.Unlock()
}

type gatedCapture struct {
	gate    *CaptureGate
	capture Capture
}

func (c *gatedCapture) Start(ctx context.Context, device string) (Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.gate.mu.Lock()
	if c.gate.occupied {
		c.gate.mu.Unlock()
		return nil, errors.New("microphone is recording in another session; stop it there first")
	}
	c.gate.occupied = true
	c.gate.mu.Unlock()

	stream, err := c.capture.Start(ctx, device)
	if err != nil {
		c.gate.release()
		return nil, err
	}
	wrapped := &gatedStream{Stream: stream, gate: c.gate}
	if err := ctx.Err(); err != nil {
		// A backend may return a stream even as startup is canceled. Do not
		// admit another recording until this one's cleanup has finished.
		_, _ = wrapped.Stop()
		return nil, err
	}
	return wrapped, nil
}

type gatedStream struct {
	Stream
	gate    *CaptureGate
	once    sync.Once
	samples []int16
	err     error
}

func (s *gatedStream) Stop() ([]int16, error) {
	s.once.Do(func() {
		s.samples, s.err = s.Stream.Stop()
		s.gate.release()
	})
	// Each caller owns its result, including callers stopping concurrently.
	return append([]int16(nil), s.samples...), s.err
}
