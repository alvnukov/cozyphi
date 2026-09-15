package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func startTestShell(t *testing.T, c *Controller, marker string) shelltask.Snapshot {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	result, err := c.shellTasks.Run(t.Context(), shelltask.Request{
		ParentSessionID: c.SessionID(), ToolUseID: marker, Command: "printf " + marker,
		Spec: proc.Spec{Argv: []string{"/bin/sh", "-c", "printf '" + marker + "'"}}, Background: true,
	})
	require.NoError(t, err)
	return result.Snapshot
}

func waitShellReceipt(t *testing.T, c *Controller, parent string) shelltask.Outcome {
	t.Helper()
	changed, unsubscribe := c.shellTasks.Subscribe()
	defer unsubscribe()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		pending, err := c.shellTasks.Pending(parent, 1)
		require.NoError(t, err)
		if len(pending) > 0 {
			return pending[0]
		}
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("shell receipt missing")
		}
	}
}

func TestShellCompletionWakesIdleSessionOnce(t *testing.T) {
	server, bodies := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(c.Close)
	task := startTestShell(t, c, "idle_shell_completed")
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !c.RunActive() })
	got := bodies()
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "idle_shell_completed")
	assert.Contains(t, got[0], "No human input has occurred")
	pending, err := c.shellTasks.Pending(c.SessionID(), 1)
	require.NoError(t, err)
	assert.Empty(t, pending)
	c.streamMu.Lock()
	generation := c.streamGen
	c.streamMu.Unlock()
	c.wakeForWatches()
	c.streamMu.Lock()
	assert.Equal(t, generation, c.streamGen, "a repeated hint cannot redeliver an acknowledged receipt")
	c.streamMu.Unlock()
	data, err := json.Marshal(c.engine.Session().PathEntries())
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), `"delivery_id":"shell:`+task.ID+`:terminal"`))
}

func TestShellBusyDeliveryUsesInboxWithoutSeparateTurn(t *testing.T) {
	server, bodies := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(c.Close)
	c.streamMu.Lock()
	c.streamRunning = true
	c.streamGen = 1
	parent := c.engine.Session()
	c.streamMu.Unlock()
	t.Cleanup(func() { c.streamMu.Lock(); c.streamRunning = false; c.streamMu.Unlock() })
	startTestShell(t, c, "busy_shell_completed")
	outcome := waitShellReceipt(t, c, c.SessionID())
	require.NoError(t, c.deliverShellOutcomes(t.Context(), 1, parent))
	require.NoError(t, c.deliverShellOutcomes(t.Context(), 1, parent))
	assert.Empty(t, bodies())
	c.streamMu.Lock()
	assert.Nil(t, c.watchWake, "a busy turn does not arm a separate wake")
	c.streamMu.Unlock()
	data, err := json.Marshal(parent.PathEntries())
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), `"delivery_id":"`+outcome.EventID+`"`))
}

func TestShellWakeBudgetKeepsReceiptForActualUserInput(t *testing.T) {
	server, bodies := textSSEServer(t)
	bus := NewBus(nil)
	c := newInjectController(t, bus, server.URL)
	t.Cleanup(c.Close)
	c.streamMu.Lock()
	c.wakeStreak = maxWakeStreak
	c.streamMu.Unlock()
	startTestShell(t, c, "budget_shell_completed")
	waitShellReceipt(t, c, c.SessionID())
	c.wakeForWatches()
	c.streamMu.Lock()
	assert.Nil(t, c.watchWake)
	assert.Zero(t, c.streamGen)
	c.streamMu.Unlock()
	require.Empty(t, bodies())
	c.StartPrompt("inspect the pending result", nil)
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !c.RunActive() })
	assert.Contains(t, bodies()[0], "budget_shell_completed")
	pending, err := c.shellTasks.Pending(c.SessionID(), 1)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestShellResultFollowsOriginalSessionAcrossClearAndResume(t *testing.T) {
	server, bodies := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(c.Close)
	original := c.SessionID()
	// A real Bash invocation always has a persisted initiating conversation.
	// This direct manager probe must establish that same session precondition.
	require.NoError(
		t,
		c.engine.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: "starting approved shell work"}),
	)
	// Keep the receipt pending while the conversation is replaced.
	c.Cancel()
	task := startTestShell(t, c, "original_session_shell")
	waitShellReceipt(t, c, original)
	require.NoError(t, c.Clear())
	require.NotEqual(t, original, c.SessionID())
	require.Len(t, c.ShellTasks(), 1, "other-session work remains visible")
	assert.Equal(t, original, c.ShellTasks()[0].ParentSessionID)
	c.StartPrompt("new conversation", nil)
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 0 && !c.RunActive() })
	assert.NotContains(t, bodies()[0], "original_session_shell")
	pending, err := c.shellTasks.Pending(original, 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	_, err = c.Resume(original)
	require.NoError(t, err)
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) > 1 && !c.RunActive() })
	assert.Contains(t, bodies()[1], "original_session_shell")
	assert.Equal(t, task.ID, c.ShellTasks()[0].ID)
}

func TestShellSnapshotsCoalesceWithoutLosingLatestState(t *testing.T) {
	bus := NewBus(nil)
	for i := range 200 {
		bus.Publish(ShellTasksChangedMsg{Tasks: []shelltask.Snapshot{{ID: "task", Output: fmt.Sprint(i)}}})
	}
	messages := bus.Drain()
	require.Len(t, messages, 1)
	snapshot, ok := messages[0].(ShellTasksChangedMsg)
	require.True(t, ok)
	assert.Equal(t, "199", snapshot.Tasks[0].Output)
}

func TestShellTurnCancellationBeforePromotionCannotBeOverridden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	server, _ := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(c.Close)
	ctx, cancel := context.WithCancel(t.Context())
	changed, unsubscribe := c.shellTasks.Subscribe()
	defer unsubscribe()
	done := make(chan error, 1)
	go func() {
		_, err := c.shellTasks.Run(ctx, shelltask.Request{
			ParentSessionID: c.SessionID(), ToolUseID: "fg",
			Command: "sleep 10", Spec: proc.Spec{Argv: []string{"/bin/sh", "-c", "sleep 10"}},
		})
		done <- err
	}()
	var id string
	for id == "" {
		<-changed
		for _, task := range c.ShellTasks() {
			id = task.ID
		}
	}
	cancel()
	require.Error(t, c.BackgroundShellTask(id), "a canceled foreground turn cannot acquire app lifetime")
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestWorkspaceOnlyReadDoesNotAdmitManagedOutputPath(t *testing.T) {
	// The safe shell_task getter owns its in-memory tail. No blanket filesystem
	// permission exception is installed for these app-private artifacts.
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	server, _ := textSSEServer(t)
	c := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(c.Close)
	c.Cancel()
	task := startTestShell(t, c, "owned_tail")
	waitShellReceipt(t, c, c.SessionID())
	foreign := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(foreign, []byte("secret"), 0o600))
	require.NoError(t, os.Remove(task.OutputFile))
	require.NoError(t, os.Symlink(foreign, task.OutputFile))
	tail, err := c.ShellTaskOutput(task.ID, 32000)
	require.NoError(t, err)
	assert.Equal(t, "owned_tail", tail)
	policy := permission.DefaultPolicy()
	policy.WorkspaceOnlyReads = true
	gate, err := permission.NewGate(policy, t.TempDir())
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]string{"path": task.OutputFile})
	require.NoError(t, err)
	req, err := permission.ExtractAt("read", raw, t.TempDir())
	require.NoError(t, err)
	decision, _ := gate.Check(t.Context(), req)
	assert.Equal(t, permission.Deny, decision, "ordinary read retains its physical-path gate")
}
