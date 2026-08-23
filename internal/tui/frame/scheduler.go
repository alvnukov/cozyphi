// Package frame decides WHEN a frame happens. It never draws: producers
// call Request/At from any goroutine, the render loop selects on Due and
// calls Frame after each completed draw.
package frame

import (
	"sync"
	"time"
)

// Scheduler collapses arbitrarily many frame requests into paced frames.
// A frame fires no sooner than minFrame after the previous one (reported
// via Frame), so high-frequency producers (streaming bursts, fast key
// repeat) are capped at the frame rate. Pending requests are min-merged:
// only the earliest deadline is armed.
type Scheduler struct {
	mu        sync.Mutex
	minFrame  time.Duration
	lastFrame time.Time
	deadline  time.Time // zero = nothing pending
	timer     *time.Timer
	due       chan struct{}
}

// New creates a Scheduler with the given minimum frame interval.
func New(minFrame time.Duration) *Scheduler {
	return &Scheduler{
		minFrame: minFrame,
		due:      make(chan struct{}, 1),
	}
}

// Request schedules a frame as soon as the throttle allows.
func (s *Scheduler) Request() {
	if s == nil {
		return
	}
	s.at(time.Now())
}

// At schedules a frame at a wall-clock time; the earliest pending deadline
// wins. A time in the past behaves like Request().
func (s *Scheduler) At(when time.Time) {
	if s == nil || when.IsZero() {
		return
	}
	s.at(when)
}

// Due fires (at most one token pending) when a frame should be drawn.
func (s *Scheduler) Due() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.due
}

// Frame records that a frame just happened, advancing the throttle window.
// If the owner forgets to call it, the scheduler fails open: Request becomes
// immediate. It never fails by losing frames.
func (s *Scheduler) Frame() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastFrame = time.Now()
	s.mu.Unlock()
}

func (s *Scheduler) at(when time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	d := effectiveDeadline(when, s.lastFrame, now, s.minFrame)
	if !s.deadline.IsZero() && !d.Before(s.deadline) {
		return // an earlier or equal deadline is already armed
	}
	s.deadline = d
	delay := time.Until(d)
	if delay < 0 {
		delay = 0
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, s.fire)
}

func (s *Scheduler) fire() {
	s.mu.Lock()
	s.deadline = time.Time{}
	s.mu.Unlock()
	select {
	case s.due <- struct{}{}:
	default: // consumer already has a token; the next draw covers us
	}
}

// effectiveDeadline clamps a requested time to the throttle window opened
// by the last completed frame.
func effectiveDeadline(requested, lastFrame, now time.Time, minFrame time.Duration) time.Time {
	d := requested
	if floor := lastFrame.Add(minFrame); d.Before(floor) {
		d = floor
	}
	if d.Before(now) {
		d = now
	}
	return d
}
