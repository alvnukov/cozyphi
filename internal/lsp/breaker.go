package lsp

import (
	"math"
	"sync"
	"time"
)

// maxStartAttempts is the number of gopls starts allowed for one root inside
// the breaker window; the next start is refused until the oldest attempt
// leaves the window.
const maxStartAttempts = 3

// startBreaker bounds gopls start attempts per canonical Go root so a crashing
// server can never become a fork bomb. Only spawn and initialize failures
// consume quota: config validation and missing-binary lookup run before the
// breaker and never record.
type startBreaker struct {
	mu       sync.Mutex
	window   time.Duration
	now      func() time.Time
	attempts map[string][]time.Time
}

func newStartBreaker(window time.Duration) *startBreaker {
	return &startBreaker{window: window, now: time.Now, attempts: make(map[string][]time.Time)}
}

// allow reports whether a start for root may proceed. A refusal carries
// retryAfter: the whole-second cool-down until the oldest attempt leaves the
// window, rounded up and never below one second.
func (b *startBreaker) allow(root string) (retryAfter time.Duration, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	attempts := b.pruneLocked(root, now)
	if len(attempts) < maxStartAttempts {
		return 0, true
	}
	remaining := attempts[0].Add(b.window).Sub(now)
	seconds := time.Duration(math.Ceil(float64(remaining) / float64(time.Second)))
	seconds = max(seconds, 1)
	return seconds * time.Second, false
}

// record notes one start attempt immediately before the process starter runs.
func (b *startBreaker) record(root string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	attempts := b.pruneLocked(root, now)
	b.attempts[root] = append(attempts, now)
}

// pruneLocked drops attempts that left the window and returns the survivors.
func (b *startBreaker) pruneLocked(root string, now time.Time) []time.Time {
	attempts := b.attempts[root]
	live := make([]time.Time, 0, len(attempts))
	for _, at := range attempts {
		if now.Sub(at) < b.window {
			live = append(live, at)
		}
	}
	b.attempts[root] = live
	return live
}
