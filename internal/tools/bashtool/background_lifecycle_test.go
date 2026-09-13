package bashtool

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func runBackgroundTask(t *testing.T, m *shelltask.Manager) shelltask.Snapshot {
	t.Helper()
	run, err := m.Run(t.Context(), shelltask.Request{
		ParentSessionID: "first", ToolUseID: "call", Command: "echo managed",
		Spec: proc.Spec{Argv: []string{"unused"}}, Background: true,
	})
	require.NoError(t, err)
	return run.Snapshot
}

func awaitTerminal(t *testing.T, m *shelltask.Manager, id string) shelltask.Snapshot {
	t.Helper()
	require.Eventually(t, func() bool {
		for _, snapshot := range m.List("first") {
			if snapshot.ID == id {
				return snapshot.State.Terminal()
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "task %s never reached a terminal state", id)
	for _, snapshot := range m.List("first") {
		if snapshot.ID == id {
			return snapshot
		}
	}
	t.Fatal("task vanished after finishing")
	return shelltask.Snapshot{}
}

// stop on a task that already reached a terminal state answers with that
// state: the terminal notification has fired, so promising a completion that
// nothing will deliver would be a lie.
func TestShellTaskStopAnswersAlreadyTerminalTask(t *testing.T) {
	m, err := shelltask.New(t.TempDir(), func(context.Context, proc.Spec, proc.Limit) (proc.Result, error) {
		return proc.Result{Output: "done"}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	snapshot := runBackgroundTask(t, m)
	awaitTerminal(t, m, snapshot.ID)
	got, err := TaskTool(
		m,
		"first",
	).Run(t.Context(), json.RawMessage(fmt.Sprintf(`{"action":"stop","id":%q}`, snapshot.ID)))
	require.NoError(t, err)
	assert.Contains(t, got.Content, "already completed")
	assert.Contains(t, got.Content, "no process to stop")
}

// A stopped task has no exit code of its own; list and get must not print one,
// or a reader would take the kill's leftover zero for success.
func TestShellTaskListAndGetOmitExitCodeForStoppedTask(t *testing.T) {
	m, err := shelltask.New(t.TempDir(), func(ctx context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		spec.Stream("tick")
		<-ctx.Done()
		return proc.Result{Canceled: true}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	snapshot := runBackgroundTask(t, m)
	require.NoError(t, m.Stop(snapshot.ID))
	stopped := awaitTerminal(t, m, snapshot.ID)
	require.Equal(t, shelltask.Stopped, stopped.State)

	got, err := TaskTool(
		m,
		"first",
	).Run(t.Context(), json.RawMessage(fmt.Sprintf(`{"action":"get","id":%q}`, snapshot.ID)))
	require.NoError(t, err)
	assert.Contains(t, got.Content, "stopped")
	assert.NotContains(t, got.Content, "exit_code", "a stopped task has no exit code to report")
}
