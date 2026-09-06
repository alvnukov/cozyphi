package controller

import "context"

type completionBarrier struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}

// TrackLifetime retains the session owner until done closes, including after a
// Close timeout. Disposal cancels all registered lifetimes before joining any.
// Register before exposing work to callers. A false result means disposal has
// started (or the barrier is invalid): the caller must cancel and deny that work.
// cancel must only signal cancellation, never wait for done or call Close.
func (c *Controller) TrackLifetime(cancel context.CancelFunc, done <-chan struct{}) bool {
	if c == nil || cancel == nil || done == nil {
		return false
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.closing {
		return false
	}
	c.lifetimes = append(c.lifetimes, completionBarrier{cancel: cancel, done: done})
	return true
}

// BeginClose denies new work and starts cancellation without waiting. The returned
// barrier closes only after workers, history ownership and runtime membership
// have been released; a Close timeout is not a substitute for this barrier.
func (c *Controller) BeginClose() <-chan struct{} {
	c.stopSession()
	return c.closeDone
}

// BeginCloseIfIdle seals admission under the same lock as prompt and assignment
// acceptance. A false result leaves work untouched so the UI can ask for consent.
func (c *Controller) BeginCloseIfIdle() bool {
	c.streamMu.Lock()
	if !c.closing && (c.streamRunning || c.switchDone != nil || len(c.promptQueue) > 0 ||
		(c.assignment != nil && !c.assignment.Terminal)) {
		c.streamMu.Unlock()
		return false
	}
	c.closing = true
	c.streamMu.Unlock()
	c.BeginClose()
	return true
}
