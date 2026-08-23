package app

import (
	"sync"
	"time"
)

// scheduler collapses arbitrarily many frame requests into paced frames.
// A frame fires no sooner than minFrame after the previous one (reported
// via frame), so high-frequency producers (streaming bursts, fast key
// repeat) are capped at the frame rate. Pending requests are min-merged:
// only the earliest deadline is armed.
type scheduler struct {
	mu        sync.Mutex
	minFrame  time.Duration
	lastFrame time.Time
	deadline  time.Time // zero = nothing pending
	timer     *time.Timer
	due       chan struct{}
}

// newScheduler creates a scheduler with the given minimum frame interval.
func newScheduler(minFrame time.Duration) *scheduler {
	return &scheduler{
		minFrame: minFrame,
		due:      make(chan struct{}, 1),
	}
}

// Request schedules a frame as soon as the throttle allows.
func (s *scheduler) Request() {
	s.at(time.Now())
}

// At schedules a frame at a wall-clock time; the earliest pending deadline
// wins. A time in the past behaves like Request().
func (s *scheduler) At(when time.Time) {
	if when.IsZero() {
		return
	}
	s.at(when)
}

// Due fires (at most one token pending) when a frame should be drawn.
func (s *scheduler) Due() <-chan struct{} {
	return s.due
}

// frame records that a frame just happened, advancing the throttle window.
// If the owner forgets to call it, the scheduler fails open: Request becomes
// immediate. It never fails by losing frames.
func (s *scheduler) frame() {
	s.mu.Lock()
	s.lastFrame = time.Now()
	s.mu.Unlock()
}

func (s *scheduler) at(when time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	d := effectiveDeadline(when, s.lastFrame, now, s.minFrame)
	if !s.deadline.IsZero() && !d.Before(s.deadline) {
		return // an earlier or equal deadline is already armed
	}
	s.deadline = d
	delay := max(time.Until(d), 0)
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, s.fire)
}

func (s *scheduler) fire() {
	s.mu.Lock()
	s.deadline = time.Time{}
	if floor := s.lastFrame.Add(s.minFrame); time.Now().Before(floor) {
		// A deadline armed mid-frame can predate the frame that just
		// landed; push the token out to the throttle floor instead of
		// breaking the pacing cap.
		s.deadline = floor
		s.timer = time.AfterFunc(max(time.Until(floor), 0), s.fire)
		s.mu.Unlock()
		return
	}
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
