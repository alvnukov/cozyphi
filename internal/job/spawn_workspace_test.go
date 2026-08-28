package job_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

// TestSpawnResolvesAndConfinesWorkdir pins the spawn boundary: with a parent
// workspace set, the workdir resolves against it, defaults to it, and may
// never escape it — the child treats the resolved workdir as its write
// boundary, so an unchecked escape would widen the parent's workspace.
func TestSpawnResolvesAndConfinesWorkdir(t *testing.T) {
	parent := t.TempDir()
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
			return "ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	info, err := mgr.Spawn(t.Context(), job.SpawnRequest{
		Prompt:          "relative",
		WorkDir:         "sub",
		ParentWorkspace: parent,
	})
	require.NoError(t, err)
	assert.Equal(
		t,
		filepath.Join(parent, "sub"),
		info.WorkDir,
		"relative workdir resolves against the parent workspace",
	)
	assert.Equal(t, parent, info.ParentWorkspace)

	info, err = mgr.Spawn(t.Context(), job.SpawnRequest{
		Prompt:          "default",
		ParentWorkspace: parent,
	})
	require.NoError(t, err)
	assert.Equal(t, parent, info.WorkDir, "empty workdir defaults to the parent workspace")

	for name, workdir := range map[string]string{
		"absolute":  t.TempDir(), // sibling of parent, outside the workspace
		"dotted-up": "../escape",
	} {
		_, err := mgr.Spawn(t.Context(), job.SpawnRequest{
			Prompt:          name,
			WorkDir:         workdir,
			ParentWorkspace: parent,
		})
		require.Error(t, err, "workdir %q must not spawn", name)
		assert.ErrorIs(t, err, job.ErrInvalid)
		assert.Contains(t, err.Error(), "outside parent workspace")
	}
}
