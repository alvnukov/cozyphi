package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestChildOutcomeWakesIdleParentWithoutWait(t *testing.T) {
	server, bodies := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(ctrl.Close)
	ctrl.runtime.EnableInteractiveChildren()
	parentID := ctrl.engine.SessionID()
	info, err := ctrl.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: "answer", OwnerID: ctrl.jobOwnerID, ParentID: parentID, ParentToolUseID: "spawn-call",
	}, job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
		if err := env.BindSession("retained-child"); err != nil {
			return "", err
		}
		return "unique child result", nil
	}))
	require.NoError(t, err)
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 })
	require.Contains(t, bodies()[0], "unique child result")
	require.Contains(t, bodies()[0], info.ID+":terminal")
	require.Contains(t, bodies()[0], "not a user instruction")
	pending, err := ctrl.jobs.PendingOutcomes(t.Context(), ctrl.jobOwnerID, parentID, 1)
	require.NoError(t, err)
	require.Empty(t, pending, "the parent context was persisted before the first inference")
}
