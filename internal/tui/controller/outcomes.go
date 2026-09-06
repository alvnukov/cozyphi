package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
)

// notifyOutcome carries only a hint; the durable job store owns the payload.
func (r *Runtime) notifyOutcome(ownerID, parentID string) {
	r.mu.Lock()
	if r.closed || !r.interactiveChildren {
		r.mu.Unlock()
		return
	}
	var parents []*Controller
	for ctrl := range r.sessions {
		if ctrl.jobOwnerID == ownerID {
			parents = append(parents, ctrl)
		}
	}
	r.mu.Unlock()
	for _, ctrl := range parents {
		ctrl.streamMu.Lock()
		if !ctrl.closing && ctrl.engine != nil && ctrl.engine.SessionID() == parentID {
			ctrl.outcomeParent = parentID
			if !ctrl.streamRunning && !ctrl.wakeSuppressed && ctrl.watchWake == nil && ctrl.wakeStreak < maxWakeStreak {
				ctrl.watchWake = time.AfterFunc(watchWakeDelay, ctrl.wakeForWatches)
			}
		}
		ctrl.streamMu.Unlock()
	}
}

func (c *Controller) hasPendingOutcomesLocked() bool {
	return c.outcomeParent != "" && c.engine != nil && c.outcomeParent == c.engine.SessionID()
}

// Both idle and final-boundary wakes reconcile receipts before starting a turn.
// A delayed producer hint is not evidence that its outcome remains unread.
func (c *Controller) reconcileOutcomeHintLocked() bool {
	if !c.hasPendingOutcomesLocked() || c.jobs == nil {
		return false
	}
	pending, err := c.jobs.PendingOutcomes(context.Background(), c.jobOwnerID, c.outcomeParent, 1)
	if err == nil && len(pending) == 0 {
		c.outcomeParent = ""
		return false
	}
	// Store errors reach the inbox boundary, which visibly reports them and
	// suppresses repeated autonomous retries without dropping the outcome.
	return true
}

func (c *Controller) deliverOutcomes(ctx context.Context, gen int, parent *agent.Session) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if gen != c.streamGen || c.streamStopped || c.closing {
		return context.Canceled
	}
	if c.jobs == nil || c.outcomeParent != parent.ID() {
		return nil
	}
	outcomes, err := c.jobs.PendingOutcomes(ctx, c.jobOwnerID, parent.ID(), 4)
	if err != nil {
		c.wakeSuppressed = true
		return err
	}
	for _, outcome := range outcomes {
		if _, err := parent.AcceptOutcome(outcome); err != nil {
			c.wakeSuppressed = true
			return fmt.Errorf("persist child result in parent context: %w", err)
		}
		// The row wants what the model just got: summary, terminal status and
		// a stopped clock, without waiting for the next resume to replay it.
		c.publish(ChildOutcomeMsg{Outcome: outcome})
		if err := c.jobs.AcknowledgeOutcome(
			ctx,
			c.jobOwnerID,
			parent.ID(),
			outcome.JobID,
			outcome.EventID,
		); err != nil {
			c.wakeSuppressed = true
			return err
		}
	}
	// Reconcile rather than dropping the hint at a batch boundary. A racing
	// producer will take streamMu after us and restore it for its new outcome.
	remaining, err := c.jobs.PendingOutcomes(ctx, c.jobOwnerID, parent.ID(), 1)
	if err != nil {
		c.wakeSuppressed = true
		return err
	}
	if len(remaining) == 0 {
		c.outcomeParent = ""
	}
	return nil
}
