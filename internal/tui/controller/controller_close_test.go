package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

// TestControllerCloseBoundedByBudget pins the shutdown budget:
// job.Manager.Close waits unconditionally for a wedged runner, so
// Controller.Close must bound its own wait — one sub-agent wedged past
// cancellation cannot hang app quit.
func TestControllerCloseBoundedByBudget(t *testing.T) {
	ready := make(chan struct{})
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
			close(ready)
			<-make(chan struct{}) // wedge past cancellation, forever
			return "", nil
		}),
	})
	require.NoError(t, err)
	_, err = mgr.Spawn(t.Context(), job.SpawnRequest{Prompt: "wedge", WorkDir: t.TempDir()})
	require.NoError(t, err)
	<-ready

	ctrl := &Controller{bus: NewBus(nil), jobs: mgr, closeBudget: 50 * time.Millisecond}

	closed := make(chan struct{})
	go func() {
		ctrl.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Controller.Close hung on a sub-agent wedged past cancellation")
	}
}
