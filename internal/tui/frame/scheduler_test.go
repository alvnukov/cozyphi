package frame

import (
	"testing"
	"time"
)

func recvToken(t *testing.T, ch <-chan struct{}, within time.Duration) bool {
	t.Helper()
	select {
	case <-ch:
		return true
	case <-time.After(within):
		return false
	}
}

func noToken(t *testing.T, ch <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected token within %v", d)
	case <-time.After(d):
	}
}

// A burst of requests before the deadline must produce exactly one token.
func TestRequestCoalesces(t *testing.T) {
	s := New(10 * time.Millisecond)
	for i := 0; i < 100; i++ {
		s.Request()
	}
	if !recvToken(t, s.Due(), 200*time.Millisecond) {
		t.Fatal("no token after burst")
	}
	noToken(t, s.Due(), 30*time.Millisecond)
}

// After a frame, an immediate request is throttled to at least minFrame.
func TestThrottlePacesFrames(t *testing.T) {
	s := New(25 * time.Millisecond)
	s.Request()
	if !recvToken(t, s.Due(), 200*time.Millisecond) {
		t.Fatal("first token missing")
	}
	s.Frame()
	start := time.Now()
	s.Request()
	if !recvToken(t, s.Due(), 200*time.Millisecond) {
		t.Fatal("second token missing")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("second frame arrived after %v, want >= minFrame-ish", elapsed)
	}
}

// The earliest pending At deadline wins; a later one must not delay it.
func TestAtEarliestWins(t *testing.T) {
	s := New(5 * time.Millisecond)
	s.At(time.Now().Add(80 * time.Millisecond))
	s.At(time.Now().Add(15 * time.Millisecond))
	start := time.Now()
	if !recvToken(t, s.Due(), 300*time.Millisecond) {
		t.Fatal("token missing")
	}
	if elapsed := time.Since(start); elapsed > 60*time.Millisecond {
		t.Fatalf("earliest deadline ignored, took %v", elapsed)
	}
}

// A deadline in the past fires immediately.
func TestAtPastIsImmediate(t *testing.T) {
	s := New(time.Hour) // throttle must not delay past deadlines on first frame
	s.At(time.Now().Add(-time.Second))
	if !recvToken(t, s.Due(), 50*time.Millisecond) {
		t.Fatal("past deadline did not fire")
	}
}

// Forgotten Frame() calls degrade to uncapped, never to lost frames.
func TestFrameForgotFailsOpen(t *testing.T) {
	s := New(time.Hour)
	s.Request()
	if !recvToken(t, s.Due(), 50*time.Millisecond) {
		t.Fatal("first token missing")
	}
	// No Frame() recorded: the next request must still fire promptly.
	s.Request()
	if !recvToken(t, s.Due(), 50*time.Millisecond) {
		t.Fatal("fail-open request lost")
	}
}

// Hammering Request/At from many goroutines must not deadlock, lose the
// final frame, or trip the race detector.
func TestConcurrentRequestAt(t *testing.T) {
	s := New(5 * time.Millisecond)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				s.Request()
				s.At(time.Now().Add(time.Duration(j%3) * time.Second))
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	s.Request()
	if !recvToken(t, s.Due(), 200*time.Millisecond) {
		t.Fatal("token lost after concurrent hammer")
	}
}

func TestEffectiveDeadlineClamp(t *testing.T) {
	now := time.Now()
	const minFrame = 50 * time.Millisecond
	cases := []struct {
		name      string
		requested time.Time
		lastFrame time.Time
		want      time.Time
	}{
		{"no frames yet honors request", now.Add(time.Second), time.Time{}, now.Add(time.Second)},
		{"clamps to throttle window", now, now.Add(10 * time.Millisecond), now.Add(60 * time.Millisecond)},
		{"past request on fresh scheduler", now.Add(-time.Hour), time.Time{}, now},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveDeadline(tc.requested, tc.lastFrame, now, minFrame)
			if !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
