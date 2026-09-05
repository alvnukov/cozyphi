package controller

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestTerminalChildFollowUpRetainsSessionAndLinksNewAssignment(t *testing.T) {
	server, bodies := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.runtime.EnableInteractiveChildren()
	parent.Cancel() // Keep parent wake inference out of this child lifecycle test.
	first, err := parent.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: "first assignment", Role: job.RoleExplore,
		OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
		WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
	}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
	require.NoError(t, err)
	waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
	child := parent.runtime.Children()[0]
	child.Ready(nil)
	old, err := parent.jobs.Wait(t.Context(), first.ID)
	require.NoError(t, err)
	require.True(t, child.Controller.Assignment().Terminal)
	require.False(t, old.Info.UserIntervened, "selecting/attaching a child is not intervention")
	child.Controller.StartPrompt("follow-up assignment", nil, "follow-up-row")
	waitForCond(t, 5*time.Second, func() bool {
		state := child.Controller.Assignment()
		return state.JobID != first.ID && state.Terminal
	})
	next, err := parent.jobs.Wait(t.Context(), child.Controller.Assignment().JobID)
	require.NoError(t, err)
	require.Equal(t, old.Info.ChildSessionID, next.Info.ChildSessionID)
	require.True(t, next.Info.UserIntervened, "the human initiated the linked follow-up")
	require.NotEmpty(t, next.Info.ChildSessionID)
	require.Equal(t, old.Info.OwnerID, next.Info.OwnerID)
	require.Equal(t, old.Info.ParentID, next.Info.ParentID)
	data, err := json.Marshal(next.Info.Meta)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	require.Equal(t, first.ID, fields["previous_job_id"])
	unchanged, err := parent.jobs.Wait(t.Context(), first.ID)
	require.NoError(t, err)
	require.Equal(t, old, unchanged, "follow-up cannot rewrite the original result")
	require.Len(t, parent.runtime.Children(), 1, "follow-up reuses the retained View")
	requests := bodies()
	require.Len(t, requests, 2)
	require.Contains(t, requests[1], "first assignment", "follow-up retains prior context")
	require.Contains(t, requests[1], "follow-up assignment")
	outcomes, err := parent.jobs.PendingOutcomes(t.Context(), parent.jobOwnerID, parent.engine.SessionID(), 4)
	require.NoError(t, err)
	require.Len(t, outcomes, 2)
	for _, outcome := range outcomes {
		if outcome.JobID == next.Info.ID {
			require.Equal(t, first.ID, outcome.PreviousJobID, "the parent receives the linkage")
			require.True(t, outcome.UserIntervened)
		} else {
			require.False(t, outcome.UserIntervened)
		}
	}
}
