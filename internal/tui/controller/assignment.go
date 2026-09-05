package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TurnState describes execution, independently from assignment completion.
type TurnState string

const (
	TurnIdle         TurnState = "idle"
	TurnRunning      TurnState = "running"
	TurnInterrupting TurnState = "interrupting"
	TurnInterrupted  TurnState = "interrupted"
)

// AssignmentSnapshot is an immutable observation of the retained assignment.
type AssignmentSnapshot struct {
	JobID         string
	Turn          TurnState
	Terminal      bool
	StopRequested bool // visible even when cancellation precedes the first stream event
}

type assignment struct {
	AssignmentSnapshot
	done       chan struct{}
	summary    string
	err        error
	stop       bool
	claimed    bool
	intervened bool
}

// Assignment reports state without consulting the selected View.
func (c *Controller) Assignment() AssignmentSnapshot {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.assignment == nil {
		return AssignmentSnapshot{}
	}
	snapshot := c.assignment.AssignmentSnapshot
	snapshot.StopRequested = c.assignment.stop
	return snapshot
}

func newAssignment(jobID string) *assignment {
	return &assignment{AssignmentSnapshot: AssignmentSnapshot{JobID: jobID, Turn: TurnIdle}, done: make(chan struct{})}
}

// RunAssignment runs through the Controller's ordinary input queue. Its caller
// owns job admission; cancellation stops the assignment, whereas Cancel only
// interrupts its current turn. Returning proves the old loop actually exited.
func (c *Controller) RunAssignment(ctx context.Context, jobID, prompt string) (string, error) {
	return c.runAssignment(ctx, jobID, queuedPrompt{text: prompt}, nil)
}

func (c *Controller) runAssignment(
	ctx context.Context,
	jobID string,
	prompt queuedPrompt,
	reportIntervention func(),
) (string, error) {
	c.streamMu.Lock()
	a := c.assignment
	if a != nil && a.JobID == jobID && !a.claimed && a.Terminal {
		// A stop can win between admission and runner entry. Consume that
		// reservation once without replacing its deliberate stop with failure.
		a.claimed = true
		c.streamMu.Unlock()
		if reportIntervention != nil && a.intervened {
			reportIntervention()
		}
		return a.summary, a.err
	}
	reserved := a != nil && a.JobID == jobID && !a.claimed && !a.Terminal
	if jobID == "" || c.closing || c.streamRunning ||
		(!reserved && (len(c.promptQueue) > 0 || (a != nil && (!a.Terminal || a.JobID == jobID)))) {
		c.streamMu.Unlock()
		return "", errors.New("cannot start assignment: require a new job ID and an idle open session")
	}
	if !reserved {
		a = newAssignment(jobID)
		c.assignment = a
	}
	a.claimed = true
	attached := c.childAttached
	c.streamMu.Unlock()
	stop := context.AfterFunc(ctx, func() { c.stopAssignment(a, ctx.Err()) })
	defer stop()
	defer func() {
		// Assignment completion fences all turn callbacks; inspect the captured
		// assignment, not a newer follow-up installed in the retained Controller.
		if reportIntervention != nil && a.intervened {
			reportIntervention()
		}
	}()
	if attached != nil {
		select {
		case <-attached.done:
			if err := attached.err; err != nil {
				c.stopAssignment(a, fmt.Errorf("attach child view: %w", err))
			}
		case <-a.done:
			return a.summary, a.err
		}
	}
	c.streamMu.Lock()
	c.childAttached = nil
	if !a.Terminal {
		if err := ctx.Err(); err != nil {
			c.stopAssignmentLocked(a, err)
		} else if c.configuredModelName() == "" {
			c.stopAssignmentLocked(a, errors.New("cannot start assignment: configure a model first"))
		} else if a.Turn == TurnIdle {
			c.startPromptLocked(prompt.text, prompt.pendingSkills, prompt.media)
		} else if a.Turn == TurnInterrupted && len(c.promptQueue) > 0 {
			// A continuation accepted during assembly replaces the interrupted
			// initial turn, but still waits for the same View readiness fence.
			next := c.promptQueue[0]
			c.promptQueue = c.promptQueue[1:]
			c.startPromptLocked(next.text, next.pendingSkills, next.media)
			if next.id != "" {
				c.publish(SessionEventMsg{Event: session.UserPromoted{ID: next.id}})
			}
		}
	}
	c.streamMu.Unlock()
	<-a.done
	return a.summary, a.err
}

// ErrLeftInterrupted records a deliberate assignment stop, not turn cancellation.
var ErrLeftInterrupted = &job.StoppedError{Reason: "left_interrupted"}

// LeaveAssignment commits stop only if continuation has not already started.
// The View calls it on actual deactivation, never for an in-session modal.
func (c *Controller) LeaveAssignment() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	a := c.assignment
	if a != nil && !a.Terminal && len(c.promptQueue) == 0 &&
		(a.Turn == TurnInterrupting || a.Turn == TurnInterrupted) {
		c.stopAssignmentLocked(a, ErrLeftInterrupted)
	}
}

func (c *Controller) stopAssignment(a *assignment, err error) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	c.stopAssignmentLocked(a, err)
}

func (c *Controller) stopAssignmentLocked(a *assignment, err error) {
	if c.assignment != a || a.Terminal {
		return
	}
	a.stop = true
	c.dropQueuedPromptsLocked()
	a.err = err
	if c.streamRunning {
		c.streamStopped = true
		a.Turn = TurnInterrupting
		if c.streamCancel != nil {
			c.streamCancel()
		}
	} else {
		c.completeAssignmentLocked()
	}
}

func (c *Controller) completeAssignmentLocked() {
	a := c.assignment
	if a == nil || a.Terminal {
		return
	}
	a.Terminal = true
	a.Turn = TurnIdle
	close(a.done)
}

func (c *Controller) recordAssignmentEvent(gen int, ev session.Event, err error) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	a := c.assignment
	if a == nil || a.Terminal || gen != c.streamGen || a.stop {
		return
	}
	if err != nil && !c.streamStopped {
		a.err = fmt.Errorf("child turn: %w", err)
	}
	if up, ok := ev.(session.AssistantMessageUpdate); ok {
		if text := strings.TrimSpace(up.Message.FlatText()); text != "" {
			a.summary = text
		}
	}
}
