package job_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

// TestCloseReapsLiveJobsAndWaitsForFinalWrites pins the Close contract: it
// cancels and reaps every live job, and the wait covers the runner's final
// writes — returning early abandons a runner mid-write and lets it write into
// directories the caller is already removing (seen as flaky TempDir cleanup
// failures in full-package runs).
func TestCloseReapsLiveJobsAndWaitsForFinalWrites(t *testing.T) {
	ready := make(chan struct{})
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			close(ready)
			<-ctx.Done()
			// Teardown writes after cancellation: these race the caller's
			// cleanup exactly when Close abandons the wait.
			time.Sleep(25 * time.Millisecond)
			return "reaped", nil
		}),
	})
	require.NoError(t, err)

	info, err := mgr.Spawn(t.Context(), job.SpawnRequest{Prompt: "reap me", WorkDir: t.TempDir()})
	require.NoError(t, err)
	<-ready

	require.NoError(t, mgr.Close())

	// The job Close cancelled reaches its terminal state deterministically,
	// and the post-cancel cancellation event is persisted before Close
	// returned — proof the wait covered the runner's final writes.
	res, err := mgr.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCancelled, res.Info.Status)

	events, err := mgr.Log(t.Context(), info.ID, 0)
	require.NoError(t, err)
	cancelled := false
	for _, ev := range events {
		if strings.HasPrefix(ev.Message, "cancelled") {
			cancelled = true
		}
	}
	assert.True(t, cancelled, "the cancellation event must be persisted before Close returns")
}
