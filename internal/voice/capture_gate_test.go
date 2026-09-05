package voice_test

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/voice"
)

// Synchronous backends need no background admission or Done watchers. Leaving
// Done open also catches a gate that leaks a watcher after explicit Stop.
func TestCaptureGateDoesNotLeaveGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	gate := voice.NewCaptureGate()
	capture := gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
		return &gateTestStream{done: make(chan struct{})}, nil
	}))
	for range 100 {
		stream, err := capture.Start(t.Context(), "device")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = stream.Stop()
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines grew from %d to %d after synchronous capture cycles", before, after)
	}
}

type gateCaptureFunc func(context.Context, string) (voice.Stream, error)

func (f gateCaptureFunc) Start(ctx context.Context, device string) (voice.Stream, error) {
	return f(ctx, device)
}

type gateTestStream struct {
	done    chan struct{}
	samples []int16
	stop    func() ([]int16, error)
}

func (*gateTestStream) Level() float64          { return 0.25 }
func (s *gateTestStream) Samples() []int16      { return append([]int16(nil), s.samples...) }
func (s *gateTestStream) Drain() []int16        { return s.Samples() }
func (*gateTestStream) Duration() time.Duration { return time.Second }
func (s *gateTestStream) Done() <-chan struct{} { return s.done }
func (s *gateTestStream) Stop() ([]int16, error) {
	if s.stop != nil {
		return s.stop()
	}
	return s.Samples(), nil
}

func gateWait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("capture operation did not finish")
	}
}

func gateDenied(t *testing.T, capture voice.Capture) {
	t.Helper()
	stream, err := capture.Start(t.Context(), "other")
	if stream != nil || err == nil || err.Error() != "microphone is recording in another session; stop it there first" {
		t.Fatalf("occupied Start = %v, %v", stream, err)
	}
}

func TestCaptureGateDeniesDuringStart(t *testing.T) {
	gate := voice.NewCaptureGate()
	entered := make(chan struct{})
	release := make(chan struct{})
	capture := gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
		close(entered)
		<-release
		return &gateTestStream{}, nil
	}))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		stream, err := capture.Start(t.Context(), "origin")
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = stream.Stop()
	}()
	defer func() { close(release); gateWait(t, finished) }()
	gateWait(t, entered)
	other := gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
		t.Error("occupied gate called other backend")
		return &gateTestStream{}, nil
	}))
	// Start itself is blocked in a backend, but denial must not wait for it.
	gateDenied(t, other)
}

func TestCaptureGateFailedAndCanceledStartRelease(t *testing.T) {
	for _, kind := range []string{"failure", "already canceled", "canceled backend", "canceled successful backend"} {
		t.Run(kind, func(t *testing.T) {
			gate := voice.NewCaptureGate()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("device unavailable")
			want := context.Canceled
			stops := 0
			if kind == "already canceled" {
				cancel()
			}
			capture := gate.Wrap(gateCaptureFunc(func(got context.Context, device string) (voice.Stream, error) {
				if got != ctx || device != "device" {
					t.Error("context/device were not forwarded")
				}
				switch kind {
				case "failure":
					return nil, failure
				case "already canceled":
					t.Error("canceled start reached backend")
				case "canceled backend":
					cancel()
					return nil, got.Err()
				}
				cancel()
				return &gateTestStream{stop: func() ([]int16, error) {
					stops++
					return nil, nil
				}}, nil
			}))
			if kind == "failure" {
				want = failure
			}
			if stream, err := capture.Start(ctx, "device"); stream != nil || !errors.Is(err, want) {
				t.Fatalf("Start = %v, %v; want %v", stream, err, want)
			}
			if kind == "canceled successful backend" && stops != 1 {
				t.Fatalf("canceled successful stream stopped %d times", stops)
			}
			next := gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
				return &gateTestStream{}, nil
			}))
			stream, err := next.Start(t.Context(), "next")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = stream.Stop()
		})
	}
}

func TestCaptureGateHoldsUntilConcurrentStopReturns(t *testing.T) {
	gate := voice.NewCaptureGate()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	close(done) // Natural completion still needs Stop's process cleanup.
	stopErr := errors.New("recording ended")
	var calls atomic.Int32
	capture := gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
		return &gateTestStream{done: done, stop: func() ([]int16, error) {
			calls.Add(1)
			close(entered)
			<-release
			return []int16{11}, stopErr
		}}, nil
	}))
	stream, err := capture.Start(t.Context(), "origin")
	if err != nil {
		t.Fatal(err)
	}
	gateWait(t, stream.Done())
	gateDenied(t, capture)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			samples, err := stream.Stop()
			if !errors.Is(err, stopErr) || !reflect.DeepEqual(samples, []int16{11}) {
				t.Errorf("Stop = %v, %v", samples, err)
			}
		})
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	defer func() { close(release); gateWait(t, finished) }()
	gateWait(t, entered)
	gateDenied(t, capture)
	if calls.Load() != 1 {
		t.Fatalf("backend Stop called %d times", calls.Load())
	}
}

func TestCaptureGateStreamsKeepOwnResults(t *testing.T) {
	gate := voice.NewCaptureGate()
	start := func(value int16, device string) voice.Stream {
		t.Helper()
		backend := &gateTestStream{samples: []int16{value}, done: make(chan struct{})}
		capture := gate.Wrap(gateCaptureFunc(func(_ context.Context, got string) (voice.Stream, error) {
			if got != device {
				t.Errorf("device = %q, want %q", got, device)
			}
			return backend, nil
		}))
		stream, err := capture.Start(t.Context(), device)
		if err != nil {
			t.Fatal(err)
		}
		if stream.Done() != backend.Done() || stream.Level() != 0.25 || stream.Duration() != time.Second ||
			!reflect.DeepEqual(
				stream.Samples(),
				backend.Samples(),
			) || !reflect.DeepEqual(stream.Drain(), backend.Drain()) {
			t.Fatal("stream methods were not preserved")
		}
		return stream
	}
	first := start(11, "one")
	samples, _ := first.Stop()
	samples[0] = 99 // A returned slice must not corrupt a later Stop result.
	second := start(22, "two")
	got, _ := first.Stop()
	if !reflect.DeepEqual(got, []int16{11}) {
		t.Fatalf("old stream result = %v", got)
	}
	// Stopping an old stream again must not release the new owner's slot.
	gateDenied(t, gate.Wrap(gateCaptureFunc(func(context.Context, string) (voice.Stream, error) {
		t.Error("old Stop released new admission")
		return &gateTestStream{}, nil
	})))
	got, _ = second.Stop()
	if !reflect.DeepEqual(got, []int16{22}) {
		t.Fatalf("new stream result = %v", got)
	}
}
