package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestRuntimeControllerJobOwnerSurvivesClearResumeAndClose(t *testing.T) {
	seed := newReadyController(t)
	t.Cleanup(seed.Close)
	rt, ws := seed.runtime, seed.workspace
	a, err := rt.NewSession(NewBus(nil), ws, "")
	require.NoError(t, err)
	b, err := rt.NewSession(NewBus(nil), ws, "")
	require.NoError(t, err)
	owner, history := a.jobOwnerID, a.SessionID()
	require.NotEmpty(t, owner)
	require.NotEqual(t, history, owner)
	require.NotEqual(t, owner, b.jobOwnerID)
	// Sessions are persisted lazily when the first assistant reply arrives.
	require.NoError(t, a.engine.Session().Append(
		llm.Message{Role: llm.RoleUser, Content: "saved history"},
		llm.Message{Role: llm.RoleAssistant, Content: "saved reply"},
	))
	historyPath := a.engine.Session().File()

	// Exercise controller teardown through the manager's public runner seam;
	// engine tool binding has separate loop-level owner tests in internal/agent.
	spawn := func(c *Controller) (job.Info, context.Context) {
		t.Helper()
		started := make(chan context.Context, 1)
		info, spawnErr := rt.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
			Prompt: "wait for controller close", ParentID: c.SessionID(), OwnerID: c.jobOwnerID,
		}, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			started <- ctx
			<-ctx.Done()
			return "", ctx.Err()
		}))
		require.NoError(t, spawnErr)
		select {
		case ctx := <-started:
			return info, ctx
		case <-time.After(5 * time.Second):
			t.Fatal("controlled job did not start")
			return job.Info{}, nil
		}
	}
	first, firstCtx := spawn(a)
	_, siblingCtx := spawn(b)
	require.Equal(t, 1, a.LiveJobCount())
	require.Equal(t, 1, b.LiveJobCount())
	require.Zero(t, seed.LiveJobCount())

	require.NoError(t, a.Clear())
	require.NotEqual(t, history, a.SessionID())
	require.Equal(t, owner, a.jobOwnerID)
	require.NoError(t, firstCtx.Err(), "Clear must not cancel the old conversation's job")
	require.Equal(t, 1, a.LiveJobCount(), "Clear must not orphan the old job count")
	second, secondCtx := spawn(a)
	require.Equal(t, 2, a.LiveJobCount())
	require.Equal(t, 1, b.LiveJobCount())

	_, err = a.Resume(history)
	require.NoError(t, err)
	require.Equal(t, history, a.SessionID())
	require.Equal(t, owner, a.jobOwnerID)
	require.Equal(t, 2, a.LiveJobCount())
	a.Close()
	a.Close()
	require.ErrorIs(t, firstCtx.Err(), context.Canceled)
	require.ErrorIs(t, secondCtx.Err(), context.Canceled)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, info := range []job.Info{first, second} {
		result, waitErr := rt.jobs.Wait(ctx, info.ID)
		require.NoError(t, waitErr)
		require.Equal(t, job.StatusCancelled, result.Info.Status)
		require.Equal(t, owner, result.Info.OwnerID)
	}
	require.Zero(t, a.LiveJobCount())
	require.NoError(t, siblingCtx.Err())
	require.Equal(t, 1, b.LiveJobCount())

	reopened, err := rt.NewSession(NewBus(nil), ws, historyPath)
	require.NoError(t, err)
	require.Equal(t, history, reopened.SessionID())
	require.NotEmpty(t, reopened.jobOwnerID)
	require.NotEqual(t, owner, reopened.jobOwnerID)
	require.NotEqual(t, b.jobOwnerID, reopened.jobOwnerID)
	require.Zero(t, reopened.LiveJobCount())
	_, reopenedCtx := spawn(reopened)
	require.Equal(t, 1, reopened.LiveJobCount())
	require.Equal(t, 1, b.LiveJobCount())
	reopened.Close()
	require.ErrorIs(t, reopenedCtx.Err(), context.Canceled)
	require.NoError(t, siblingCtx.Err(), "closing a reopened history must not close sibling owners")
	b.Close()
	require.ErrorIs(t, siblingCtx.Err(), context.Canceled)
}
