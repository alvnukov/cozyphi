package job_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestUndeliveredOutcomeRetainsBoundedCapacityUntilStorageRecovers(t *testing.T) {
	root := t.TempDir()
	hint := make(chan struct{}, 1)
	runner := job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
		// Block only the final metadata replacement, leaving the old record readable.
		if err := os.Mkdir(filepath.Join(env.Job.Dir, "meta.json.tmp"), 0o700); err != nil {
			return "", err
		}
		return "retained despite failed persistence", nil
	})
	manager, err := job.New(job.Options{
		Root: root, Runner: runner, MaxConcurrent: 1,
		OnOutcome: func(string, string) {
			select {
			case hint <- struct{}{}:
			default:
			}
		},
	})
	require.NoError(t, err)
	defer manager.Close()
	req := job.SpawnRequest{Prompt: "work", OwnerID: "owner", ParentID: "parent"}
	info, err := manager.Spawn(t.Context(), req)
	require.NoError(t, err)
	select {
	case <-hint:
	case <-time.After(3 * time.Second):
		t.Fatal("failed delivery must notify its parent")
	}
	require.NoError(t, manager.Cancel(t.Context(), info.ID)) // Reap the actual runner exit.
	_, err = manager.Wait(t.Context(), info.ID)
	require.ErrorContains(t, err, "undelivered outcome")
	_, err = manager.PendingOutcomes(t.Context(), "owner", "parent", 1)
	require.ErrorContains(t, err, "undelivered outcome")
	_, err = manager.Spawn(t.Context(), req)
	require.ErrorIs(t, err, job.ErrBusy, "failed persistence must not grow an unbounded memory backlog")
	require.NoError(t, os.Remove(filepath.Join(info.Dir, "meta.json.tmp")))
	outcomes, err := manager.PendingOutcomes(t.Context(), "owner", "parent", 1)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.Equal(t, "retained despite failed persistence", outcomes[0].Summary)
	waited, err := manager.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	require.Equal(t, outcomes[0].EventID, waited.Info.OutcomeID)
	_, err = manager.SpawnWithRunner(
		t.Context(),
		req,
		job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "next", nil }),
	)
	require.NoError(t, err, "reconciliation releases the quarantined admission slot")
}
