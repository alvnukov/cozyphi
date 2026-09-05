package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
)

// startFollowUpLocked admits a fresh assignment, never reviving the terminal one.
// The existing manager bounds execution and the existing Controller queues input;
// neither the retained View nor its engine is reconstructed.
func (c *Controller) startFollowUpLocked(prompt queuedPrompt) {
	manager := c.runtime.jobs
	previous, err := manager.Get(context.Background(), c.assignment.JobID)
	if err != nil {
		c.refuseFollowUp(err)
		return
	}
	sessionID := c.engine.SessionID()
	runner := followUpRunner{controller: c, sessionID: sessionID, prompt: prompt}
	next, err := manager.SpawnWithRunner(context.Background(), job.SpawnRequest{
		Prompt: prompt.text, Description: previous.Description,
		PreviousJobID: previous.ID, OwnerID: previous.OwnerID, ParentID: previous.ParentID,
		Role: previous.Role, Depth: previous.ParentDepth,
		WorkDir: previous.WorkDir, ParentWorkspace: previous.ParentWorkspace,
	}, runner)
	if err != nil {
		c.refuseFollowUp(err)
		return
	}
	// The runner takes streamMu before executing, so reservation wins publication.
	c.assignment = newAssignment(next.ID)
	c.assignment.intervened = true
}

type followUpRunner struct {
	controller *Controller
	sessionID  string
	prompt     queuedPrompt
}

func (r followUpRunner) Run(ctx context.Context, env job.RunEnv) (string, error) {
	if env.MarkIntervention != nil {
		env.MarkIntervention() // The human initiated this assignment, even if binding fails.
	}
	if err := env.BindSession(r.sessionID); err != nil {
		c := r.controller
		c.streamMu.Lock()
		if a := c.assignment; a != nil && a.JobID == env.Job.ID && !a.Terminal {
			c.stopAssignmentLocked(a, err)
			c.refuseFollowUp(err)
		}
		c.streamMu.Unlock()
		return "", err
	}
	return r.controller.runAssignment(ctx, env.Job.ID, r.prompt, env.MarkIntervention)
}

func (c *Controller) refuseFollowUp(err error) {
	text := fmt.Sprintf("Cannot start child follow-up: %v. Resolve the error and resubmit your message.", err)
	c.publish(SessionEventMsg{Event: session.AssistantMessageUpdate{Message: session.Message{
		ID: fmt.Sprintf("follow-up-error-%d", time.Now().UnixNano()), State: session.StateError,
		Text: text, Content: []session.ContentBlock{{Type: session.BlockText, Text: text}},
	}}})
	c.publish(RunEndedMsg{})
}
