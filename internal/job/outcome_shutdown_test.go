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

func TestShutdownReportsAndReconcilesUndeliveredOutcomes(t *testing.T) {
	for _, scope := range []string{"manager", "owner", "parent"} {
		t.Run(scope, func(t *testing.T) {
			hint := make(chan struct{}, 1)
			manager, err := job.New(job.Options{
				Root: t.TempDir(),
				Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
					if err := os.Mkdir(filepath.Join(env.Job.Dir, "meta.json.tmp"), 0o700); err != nil {
						return "", err
					}
					return "shutdown must retain this result", nil
				}),
				OnOutcome: func(string, string) { hint <- struct{}{} },
			})
			require.NoError(t, err)
			defer manager.Close()
			info, err := manager.Spawn(
				t.Context(),
				job.SpawnRequest{Prompt: "work", OwnerID: "owner", ParentID: "parent"},
			)
			require.NoError(t, err)
			select {
			case <-hint:
			case <-time.After(3 * time.Second):
				t.Fatal("outcome hint missing")
			}
			closeScope := func() error {
				switch scope {
				case "owner":
					return manager.CloseOwner(t.Context(), "owner")
				case "parent":
					return manager.CloseParent(t.Context(), "parent")
				default:
					return manager.Close()
				}
			}
			require.NoError(t, manager.CloseOwner(t.Context(), "other-owner"), "foreign errors are not ours to close")
			require.NoError(t, manager.CloseParent(t.Context(), "other-parent"))
			err = closeScope()
			require.ErrorContains(t, err, "undelivered outcome")
			require.ErrorContains(t, err, info.ID)
			require.NoError(t, os.Remove(filepath.Join(info.Dir, "meta.json.tmp")))
			require.NoError(t, closeScope(), "repeated close retries final persistence without running the job again")
			outcomes, err := manager.PendingOutcomes(t.Context(), "owner", "parent", 1)
			require.NoError(t, err)
			require.Len(t, outcomes, 1)
			require.Equal(t, "shutdown must retain this result", outcomes[0].Summary)
		})
	}
}
