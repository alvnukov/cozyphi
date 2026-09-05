package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestOwnerAccessUsesTrustedMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	m := newMgr(t, job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
		env.Log("private log")
		<-ctx.Done()
		return "", ctx.Err()
	}), job.Options{})
	for _, owner := range []string{"a", "b", ""} {
		info, err := m.Spawn(t.Context(), job.SpawnRequest{
			Prompt: "private", OwnerID: owner, ParentID: "shared",
		})
		require.NoError(t, err)
		for _, other := range []string{"a", "b"} {
			if other == owner {
				continue
			}
			_, err = m.WaitForOwner(ctx, info.ID, other)
			require.ErrorIs(t, err, job.ErrNotFound)
			_, err = m.LogForOwner(t.Context(), info.ID, 0, other)
			require.ErrorIs(t, err, job.ErrNotFound)
			require.ErrorIs(t, m.CancelForOwner(t.Context(), info.ID, other), job.ErrNotFound)
		}
	}
	for _, owner := range []string{"a", "b"} {
		list, err := m.ListForOwner(t.Context(), owner)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, owner, list[0].OwnerID)
		require.NoError(t, m.CancelForOwner(t.Context(), list[0].ID, owner))
		result, err := m.WaitForOwner(ctx, list[0].ID, owner)
		require.NoError(t, err)
		assert.Equal(t, job.StatusCancelled, result.Info.Status)
		logs, err := m.LogForOwner(t.Context(), list[0].ID, 0, owner)
		require.NoError(t, err)
		require.NotEmpty(t, logs)
		_, err = m.LogForOwner(t.Context(), list[0].ID, 0, "not-owner")
		require.ErrorIs(t, err, job.ErrNotFound, "persisted terminal metadata remains authoritative")
	}
	all, err := m.ListForOwner(t.Context(), "")
	require.NoError(t, err)
	assert.Len(t, all, 3, "empty tool scope preserves legacy global access")
}
