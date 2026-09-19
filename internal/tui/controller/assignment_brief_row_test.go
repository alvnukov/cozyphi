package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
)

// userRows drains a bus and returns the text of every user transcript row it
// carried, in order — what the screen built from this bus would show.
func userRows(bus *Bus) []string {
	var out []string
	for _, m := range bus.Drain() {
		ev, ok := m.(SessionEventMsg)
		if !ok {
			continue
		}
		if appended, ok := ev.Event.(session.UserAppend); ok {
			out = append(out, appended.Text)
		}
	}
	return out
}

// spawnRunningChild starts one interactive child and waits for its assignment
// to finish, returning the retained child and the job it ran.
func spawnRunningChild(t *testing.T, parent *Controller, description, prompt string) (ChildSession, string) {
	t.Helper()
	spawned, err := parent.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: prompt, Description: description, Role: job.RoleExplore,
		OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
		WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
	}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
	require.NoError(t, err)
	waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
	child := parent.runtime.Children()[0]
	child.Ready(nil)
	_, err = parent.jobs.Wait(t.Context(), spawned.ID)
	require.NoError(t, err)
	return child, spawned.ID
}

// A sub-agent's screen must open on the brief that started it. The child's
// first user message is assembled rather than typed, so no composer publishes
// a row for it — and without one the screen starts mid-way, on the model's
// first move, with the instructions it is following nowhere on the screen.
func TestAssignmentBriefOpensTheChildTranscript(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.runtime.EnableInteractiveChildren()
	parent.Cancel() // Keep parent wake inference out of this child lifecycle test.
	child, _ := spawnRunningChild(t, parent, "queue cleanup", "refactor the queue")

	rows := userRows(child.Bus)
	require.Len(t, rows, 1, "the assignment opens the child transcript with exactly one row")
	require.Contains(t, rows[0], "refactor the queue")
	require.Contains(t, rows[0], "queue cleanup", "the description opens the brief")
	require.Contains(t, rows[0], agent.SpecForRole(job.RoleExplore).Hint,
		"the row is the whole first message, as a replay of this session would show it")
}

// A human follow-up into a retained child gets exactly one row, and it is the
// engine that draws it when the new assignment delivers the prompt: the
// composer publishes nothing for a queued submit, and the brief that opens a
// child transcript is only for the assembled first message.
func TestChildFollowUpKeepsOneUserRow(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.runtime.EnableInteractiveChildren()
	parent.Cancel()
	child, first := spawnRunningChild(t, parent, "queue cleanup", "refactor the queue")
	require.Len(t, userRows(child.Bus), 1)

	queued, _ := child.Controller.StartPrompt("follow-up assignment", nil)
	require.True(t, queued)
	waitForCond(t, 5*time.Second, func() bool {
		state := child.Controller.Assignment()
		return state.JobID != first && state.Terminal
	})
	var appended, promoted []string
	for _, m := range child.Bus.Drain() {
		ev, ok := m.(SessionEventMsg)
		if !ok {
			continue
		}
		switch event := ev.Event.(type) {
		case session.UserAppend:
			appended = append(appended, event.Text)
		case session.UserPromoted:
			promoted = append(promoted, event.Text)
		}
	}
	require.Empty(t, appended, "no brief opens a follow-up: the human typed this one")
	require.Equal(t, []string{"follow-up assignment"}, promoted,
		"the row lands when the assignment actually delivers the prompt")
}
