package lsp

import (
	"testing"
	"time"
)

// newTestBreaker wires a breaker to a stepped clock so window math is exact.
func newTestBreaker() (*startBreaker, func(time.Duration)) {
	b := newStartBreaker(60 * time.Second)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	return b, func(d time.Duration) { now = now.Add(d) }
}

// TestBreakerAllowsThreeRefusesFourth proves three recorded attempts exhaust a
// root and the fourth start is refused with the full window as cool-down.
func TestBreakerAllowsThreeRefusesFourth(t *testing.T) {
	b, _ := newTestBreaker()
	for range maxStartAttempts {
		if _, ok := b.allow("root"); !ok {
			t.Fatal("attempt within quota refused")
		}
		b.record("root")
	}
	retryAfter, ok := b.allow("root")
	if ok {
		t.Fatal("fourth start must be refused")
	}
	if retryAfter != 60*time.Second {
		t.Fatalf("retryAfter = %s, want 60s", retryAfter)
	}
}

// TestBreakerRetryAfterCeilsToOneSecondMinimum proves a fractional remaining
// window rounds up and never reports a zero cool-down.
func TestBreakerRetryAfterCeilsToOneSecondMinimum(t *testing.T) {
	b, step := newTestBreaker()
	for range maxStartAttempts {
		b.record("root")
	}
	step(59*time.Second + 400*time.Millisecond) // 0.4s left in the window
	retryAfter, ok := b.allow("root")
	if ok {
		t.Fatal("start must still be refused")
	}
	if retryAfter != 1*time.Second {
		t.Fatalf("retryAfter = %s, want 1s (ceil of 0.4s)", retryAfter)
	}
}

// TestBreakerRetryAfterRoundsUp proves 1.4s remaining becomes a 2s hint.
func TestBreakerRetryAfterRoundsUp(t *testing.T) {
	b, step := newTestBreaker()
	for range maxStartAttempts {
		b.record("root")
	}
	step(58*time.Second + 600*time.Millisecond) // 1.4s left
	retryAfter, ok := b.allow("root")
	if ok {
		t.Fatal("start must still be refused")
	}
	if retryAfter != 2*time.Second {
		t.Fatalf("retryAfter = %s, want 2s", retryAfter)
	}
}

// TestBreakerWindowSlidesProvesRecovery: once the oldest attempt leaves the
// window the same root becomes eligible again.
func TestBreakerWindowSlidesProvesRecovery(t *testing.T) {
	b, step := newTestBreaker()
	for range maxStartAttempts {
		b.record("root")
	}
	step(60*time.Second + time.Millisecond)
	if _, ok := b.allow("root"); !ok {
		t.Fatal("start must be allowed after the window slid")
	}
}

// TestBreakerRootsAreIndependent proves quota is tracked per canonical root.
func TestBreakerRootsAreIndependent(t *testing.T) {
	b, _ := newTestBreaker()
	for range maxStartAttempts {
		b.record("/ws/a")
	}
	if _, ok := b.allow("/ws/a"); ok {
		t.Fatal("exhausted root must refuse")
	}
	if _, ok := b.allow("/ws/b"); !ok {
		t.Fatal("a different root must keep its own quota")
	}
}
