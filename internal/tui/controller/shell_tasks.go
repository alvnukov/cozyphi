package controller

import (
	"context"
	"errors"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// ShellTasksChangedMsg carries a detached full snapshot for user-facing views.
// The bus coalesces it; the durable terminal inbox is independent of this message.
type ShellTasksChangedMsg struct{ Tasks []shelltask.Snapshot }

func (ShellTasksChangedMsg) isMsg() {}

// ReplayShellTasks projects persisted launch and terminal receipts separately
// from tool invocation status. Runtime snapshots take precedence in the view.
func (c *Controller) ReplayShellTasks() []shelltask.Snapshot {
	if c == nil || c.engine == nil || c.engine.Session() == nil {
		return nil
	}
	return transcript.ReplayShellTasks(c.engine.Session().PathEntries())
}

// ShellTasks shows all shell work owned by this application, including other conversations.
func (c *Controller) ShellTasks() []shelltask.Snapshot {
	if c == nil || c.shellTasks == nil {
		return nil
	}
	return c.shellTasks.List("")
}

// BackgroundShellTask is an explicit user action, never exposed to the model.
func (c *Controller) BackgroundShellTask(id string) error {
	if c == nil || c.shellTasks == nil {
		return errors.New("background shell tasks are unavailable")
	}
	return c.shellTasks.Background(id)
}

// StopShellTask requests cancellation without waiting for process cleanup on the UI.
func (c *Controller) StopShellTask(id string) error {
	if c == nil || c.shellTasks == nil {
		return errors.New("background shell tasks are unavailable")
	}
	return c.shellTasks.Stop(id)
}

// ShellTaskOutput returns a bounded in-memory tail; limit is bytes, capped at 32 KiB.
func (c *Controller) ShellTaskOutput(id string, limit int) (string, error) {
	if c == nil || c.shellTasks == nil {
		return "", errors.New("background shell tasks are unavailable")
	}
	return c.shellTasks.Output(id, limit)
}

func (c *Controller) startShellTaskEvents() {
	if c.shellTasks == nil || c.bus == nil {
		return
	}
	changed, cancel := c.shellTasks.Subscribe()
	c.unsubShellTasks = cancel
	go func() {
		for range changed {
			c.streamMu.Lock()
			if c.closing {
				c.streamMu.Unlock()
				return
			}
			c.publish(ShellTasksChangedMsg{Tasks: c.ShellTasks()})
			if c.switchDone == nil && !c.streamRunning && !c.wakeSuppressed &&
				c.watchWake == nil && c.wakeStreak < maxWakeStreak && c.hasPendingShellOutcomesLocked() {
				c.watchWake = time.AfterFunc(watchWakeDelay, c.wakeForWatches)
			}
			c.streamMu.Unlock()
		}
	}()
}

// Only the original conversation may consume a terminal receipt. An unrelated
// tab can inspect/stop the process, but cannot receive its output as model input.
func (c *Controller) hasPendingShellOutcomesLocked() bool {
	if c.shellTasks == nil || c.engine == nil || c.childRole != "" {
		return false
	}
	outcomes, err := c.shellTasks.Pending(c.engine.SessionID(), 1)
	return err != nil || len(outcomes) > 0
}

func (c *Controller) deliverShellOutcomes(ctx context.Context, gen int, parent *agent.Session) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if gen != c.streamGen || c.streamStopped || c.closing {
		return context.Canceled
	}
	if c.shellTasks == nil {
		return nil
	}
	outcomes, err := c.shellTasks.Pending(parent.ID(), 4)
	if err != nil {
		c.wakeSuppressed = true
		return err
	}
	for _, outcome := range outcomes {
		if _, err := parent.AcceptShellOutcome(outcome); err != nil {
			c.wakeSuppressed = true
			return err
		}
		if err := c.shellTasks.Acknowledge(parent.ID(), outcome.EventID); err != nil {
			c.wakeSuppressed = true
			return err
		}
	}
	return nil
}
