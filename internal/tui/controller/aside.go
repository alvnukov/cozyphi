package controller

import (
	"context"
	"errors"

	"github.com/alvnukov/cozyphi/internal/session"
)

// Aside asks a side question about the context up to the anchor, or about
// all of it when the anchor is empty, and streams the answer onto the bus as
// session.AsideUpdate events. It waits for the same idle pipeline a rewind
// does, and every refusal comes back from here, before the model is asked.
//
// The question holds the pipeline while it is answered, like /compact: a
// prompt sent meanwhile queues behind it, and Esc cancels it.
func (c *Controller) Aside(question, anchorID string) error {
	if c == nil || c.engine == nil {
		return errors.New("controller: no engine")
	}
	c.streamMu.Lock()
	if err := c.requireRunIdleLocked("ask a side question"); err != nil {
		c.streamMu.Unlock()
		return err
	}
	if c.configuredModelName() == "" {
		c.streamMu.Unlock()
		return errors.New("cannot ask a side question: no model is configured")
	}
	engine := c.engine
	req, err := engine.PrepareAside(question, anchorID)
	if err != nil {
		c.streamMu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.streamRunning = true
	c.streamStopped = false
	c.streamGen++
	gen := c.streamGen
	c.streamCancel = cancel
	c.streamWG.Go(func() {
		defer cancel()
		defer c.finishRun(gen)
		// The outcome is on the row: the engine ends it complete, cancelled
		// or with the error, so the returned error adds nothing to show.
		_ = engine.RunAside(ctx, req, func(ev session.Event) bool {
			if !c.Alive(gen) {
				// Esc stops the stream but must not strand the row: the
				// update that ends it is the only thing that settles its
				// state, so it still goes out while this run is the
				// current one.
				if update, ok := ev.(session.AsideUpdate); ok &&
					update.State != session.StateStreaming && c.sameGeneration(gen) {
					c.publish(SessionEventMsg{Event: ev})
				}
				return false
			}
			c.publish(SessionEventMsg{Event: ev})
			return true
		})
	})
	c.streamMu.Unlock()
	c.publish(SetActivityMsg{Activity: ActivityStreaming})
	return nil
}

// sameGeneration reports whether the run gen started is still the current one,
// stopped or not.
func (c *Controller) sameGeneration(gen int) bool {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	return c.streamGen == gen
}

// AsideAnchors lists the messages a side question may be asked about, oldest
// first. Asking changes nothing, so it needs no busy guard.
func (c *Controller) AsideAnchors() []session.AsideAnchor {
	if c == nil || c.engine == nil {
		return nil
	}
	return c.engine.AsideAnchors()
}
