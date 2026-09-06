package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

// TestCancelChildStopsThroughTheOwnedManagerPath pins the panel's x to the very
// seam agent_cancel uses, ownership check included: the two must not diverge.
func TestCancelChildStopsThroughTheOwnedManagerPath(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	started := make(chan struct{})
	blocking := job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "long assignment", Role: job.RoleExplore,
		OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
		WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
	}, blocking)
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, parent.CancelChild(ctx, info.ID))
	result, err := parent.jobs.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCancelled, result.Info.Status)
}

func TestCancelChildRefusesAnotherOwnersJob(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	started := make(chan struct{})
	blocking := job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "someone else's assignment", Role: job.RoleExplore,
		OwnerID: "another-session", ParentID: "another-session",
		WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
	}, blocking)
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.ErrorIs(t, parent.CancelChild(ctx, info.ID), job.ErrNotFound)
	require.NoError(t, parent.jobs.CancelForOwner(ctx, info.ID, "another-session"))
}

func TestCancelChildWithoutManagerReportsUnavailable(t *testing.T) {
	require.ErrorContains(t, (&Controller{}).CancelChild(t.Context(), "job"), "not available")
}
