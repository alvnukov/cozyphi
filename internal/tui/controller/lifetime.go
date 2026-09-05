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
