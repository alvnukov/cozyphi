package job_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestPendingOutcomesIsolatesForeignAndUnpublishedMetadata(t *testing.T) {
	root := t.TempDir()
	manager, err := job.New(
		job.Options{
			Root:   root,
			Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "answer", nil }),
		},
	)
	require.NoError(t, err)
	defer func() { _ = manager.Close() }()
	own, err := manager.Spawn(t.Context(), job.SpawnRequest{Prompt: "own", OwnerID: "owner-a", ParentID: "parent-a"})
	require.NoError(t, err)
	_, err = manager.Wait(t.Context(), own.ID)
	require.NoError(t, err)
	other, err := manager.Spawn(
		t.Context(),
		job.SpawnRequest{Prompt: "other", OwnerID: "owner-b", ParentID: "parent-b"},
	)
	require.NoError(t, err)
	_, err = manager.Wait(t.Context(), other.ID)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(other.Dir, "meta.json"), []byte("broken"), 0o600))
	// A concurrent create has made its directory, not yet its metadata.
	require.NoError(t, os.Mkdir(filepath.Join(root, "job_unpublished"), 0o700))
	pending, err := manager.PendingOutcomes(t.Context(), "owner-a", "parent-a", 4)
	require.NoError(t, err, "foreign corruption and incomplete admission must not disable this parent")
	require.Len(t, pending, 1)
	require.Equal(t, own.ID, pending[0].JobID)
	require.NoError(t, os.WriteFile(filepath.Join(own.Dir, "meta.json"), []byte("broken"), 0o600))
	_, err = manager.PendingOutcomes(t.Context(), "owner-a", "parent-a", 4)
	require.ErrorContains(t, err, own.ID, "known owned corruption must remain actionable")
}
