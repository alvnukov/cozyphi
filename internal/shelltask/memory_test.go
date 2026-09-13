package shelltask

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
)

// Retained heap is an ownership invariant with no public getter: checking the
// entry here catches accidental duplicate full-output retention without a flaky
// allocator/GC benchmark.
func TestBackgroundHistoryDoesNotRetainFullProcessResult(t *testing.T) {
	m, err := New(t.TempDir(), func(_ context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		output := strings.Repeat("large output ", OutputFileBytes/13)
		spec.Stream(output)
		return proc.Result{Output: output}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	result, err := m.Run(t.Context(), Request{ParentSessionID: "p", ToolUseID: "t", Command: "large", Background: true})
	require.NoError(t, err)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		pending, err := m.Pending("p", 1)
		require.NoError(t, err)
		if len(pending) > 0 {
			require.NoError(t, m.Acknowledge("p", pending[0].EventID))
			break
		}
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("missing result")
		}
	}
	m.mu.Lock()
	retained := m.entries[result.Snapshot.ID].result.Output
	tail := m.entries[result.Snapshot.ID].snapshot.Output
	m.mu.Unlock()
	require.Empty(t, retained)
	require.LessOrEqual(t, len(tail), TailBytes)
	require.Empty(t, result.Process.Output)
}
