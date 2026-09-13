package shelltask_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func TestHistoryPressureNeverBlocksForegroundOrDiscardsPending(t *testing.T) {
	m := newManager(t, func(context.Context, proc.Spec, proc.Limit) (proc.Result, error) {
		return proc.Result{Output: "full foreground result"}, nil
	})
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	var first shelltask.Outcome
	for i := range shelltask.MaxTasks {
		result, err := m.Run(t.Context(), request(fmt.Sprintf("background-%d", i), true))
		require.NoError(t, err)
		deadline := time.NewTimer(5 * time.Second)
		for {
			pending, err := m.Pending("parent", shelltask.MaxTasks)
			require.NoError(t, err)
			if len(pending) == i+1 {
				if i == 0 {
					first = pending[0]
				}
				break
			}
			select {
			case <-changed:
			case <-deadline.C:
				t.Fatal("completion missing")
			}
		}
		deadline.Stop()
		assert.True(t, result.Snapshot.Background)
	}
	_, err := m.Run(t.Context(), request("blocked-background", true))
	require.ErrorContains(t, err, "undelivered")
	result, err := m.Run(t.Context(), request("foreground", false))
	require.NoError(t, err)
	assert.Equal(t, "full foreground result", result.Process.Output)
	assert.Len(t, m.List("parent"), shelltask.MaxTasks, "foreground does not consume history")
	_, err = os.Stat(filepath.Dir(result.Snapshot.OutputFile))
	assert.True(t, os.IsNotExist(err), "foreground artifacts retire with its native result")
	require.NoError(t, m.Acknowledge("parent", first.EventID))
	_, err = m.Run(t.Context(), request("replacement", true))
	require.NoError(t, err, "only delivered terminal history may be evicted")
	assert.Len(t, m.List("parent"), shelltask.MaxTasks)
	for _, snapshot := range m.List("parent") {
		assert.NotEqual(t, first.ID, snapshot.ID)
	}
	require.NoError(t, m.Close())
	_, err = os.Stat(filepath.Dir(first.OutputFile))
	assert.True(t, os.IsNotExist(err), "Close joins bounded artifact cleanup")
	pending, err := m.Pending("parent", shelltask.MaxTasks)
	require.NoError(t, err)
	assert.Len(t, pending, shelltask.MaxTasks, "all unacknowledged results survive pressure")
}

func TestRepeatedForegroundRunsReleaseHistoryCapacity(t *testing.T) {
	m := newManager(t, func(context.Context, proc.Spec, proc.Limit) (proc.Result, error) {
		return proc.Result{Output: "ok"}, nil
	})
	for i := range shelltask.MaxTasks + 1 {
		result, err := m.Run(t.Context(), request(fmt.Sprintf("foreground-%d", i), false))
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Process.Output)
	}
	assert.Empty(t, m.List(""))
}
