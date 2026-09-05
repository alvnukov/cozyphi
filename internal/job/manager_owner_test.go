package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestCloseOwnerSeparatesLifetimeFromHistory(t *testing.T) {
	started := make(chan context.Context, 3)
	m := newMgr(t, job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
		started <- ctx
		<-ctx.Done()
		return "", env.WriteResult("final write")
	}), job.Options{})
	var ids []string
	for _, parent := range []string{"before-resume", "after-resume"} {
		info, err := m.Spawn(t.Context(), job.SpawnRequest{
			Prompt: "old", ParentID: parent, OwnerID: "old-owner",
		})
		require.NoError(t, err)
		ids = append(ids, info.ID)
		receiveJobSignal(t, started)
	}
	assert.Equal(t, 2, m.LiveCountForOwner("old-owner"))
	require.NoError(t, m.CloseOwner(t.Context(), "old-owner"))
	assert.Zero(t, m.LiveCountForOwner("old-owner"))
	for _, id := range ids {
		result, err := m.Wait(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, "old-owner", result.Info.OwnerID)
		assert.Equal(t, job.StatusCancelled, result.Info.Status)
		assert.Equal(t, "final write", result.Summary)
	}
	_, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "rejected", OwnerID: "old-owner"})
	require.ErrorIs(t, err, job.ErrClosed)
	_, err = m.Spawn(t.Context(), job.SpawnRequest{
		Prompt: "new", ParentID: "after-resume", OwnerID: "new-owner",
	})
	require.NoError(t, err)
	newCtx := receiveJobSignal(t, started)
	require.NoError(t, m.CloseOwner(t.Context(), "old-owner"))
	assert.NoError(t, newCtx.Err())
	assert.Equal(t, 1, m.LiveCountForOwner("new-owner"))
}

func TestCloseOwnerTimedOutAdmissionCannotCancelReplacement(t *testing.T) {
	started := make(chan context.Context, 1)
	m := newMgr(t, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		started <- ctx
		<-ctx.Done()
		return "", ctx.Err()
	}), job.Options{MaxConcurrent: 2})
	admitting := &admissionContext{
		Context: t.Context(), entered: make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(func() { close(admitting.release) })
	spawned := make(chan error, 1)
	go func() {
		_, err := m.Spawn(admitting, job.SpawnRequest{
			Prompt: "old", ParentID: "same-history", OwnerID: "old",
		})
		spawned <- err
	}()
	receiveJobSignal(t, admitting.entered)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.CloseOwner(ctx, "old"), context.DeadlineExceeded)
	assert.Equal(t, 1, m.LiveCountForOwner("old"), "pending admission still owns a slot")
	replacement, err := m.Spawn(t.Context(), job.SpawnRequest{
		Prompt: "new", ParentID: "same-history", OwnerID: "new",
	})
	require.NoError(t, err)
	newCtx := receiveJobSignal(t, started)
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "full", OwnerID: "new"})
	require.ErrorIs(t, err, job.ErrBusy)
	admitting.release <- struct{}{}
	require.ErrorIs(t, receiveJobSignal(t, spawned), job.ErrClosed)
	require.NoError(t, m.CloseOwner(t.Context(), "old"))
	assert.NoError(t, newCtx.Err())
	info, err := m.Get(t.Context(), replacement.ID)
	require.NoError(t, err)
	assert.Equal(t, "new", info.OwnerID)
}

func TestCloseOwnerTimeoutKeepsSlotUntilFinalWrite(t *testing.T) {
	cancelled := make(chan struct{})
	release := make(chan struct{})
	m := newMgr(t, job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
		<-ctx.Done()
		close(cancelled)
		<-release
		return "", env.WriteResult("owner teardown finished")
	}), job.Options{MaxConcurrent: 1})
	t.Cleanup(func() { close(release) })
	info, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "old", ParentID: "history", OwnerID: "old"})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.CloseOwner(ctx, "old"), context.DeadlineExceeded)
	receiveJobSignal(t, cancelled)
	assert.Equal(t, 1, m.LiveCountForOwner("old"))
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "new", ParentID: "history", OwnerID: "new"})
	require.ErrorIs(t, err, job.ErrBusy)
	release <- struct{}{}
	require.NoError(t, m.CloseOwner(t.Context(), "old"))
	assert.Zero(t, m.LiveCountForOwner("old"))
	result, err := m.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCancelled, result.Info.Status)
	assert.Equal(t, "owner teardown finished", result.Summary)
}
