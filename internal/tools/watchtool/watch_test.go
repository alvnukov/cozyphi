package watchtool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/watchtool"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// newTool builds the tool over a manager with a scripted shell and returns
// the one thing a test needs: a way to call it the way the model does.
func newTool(t *testing.T, shell watch.ShellFunc) func(string) (string, error) {
	t.Helper()
	mgr := watch.New(watch.Options{Shell: shell})
	t.Cleanup(mgr.Close)
	tool := watchtool.Tool(watchtool.Deps{Manager: mgr})
	return func(args string) (string, error) {
		res, err := tool.Run(t.Context(), json.RawMessage(args))
		return res.Content, err
	}
}

func blockingShell() watch.ShellFunc {
	return func(ctx context.Context, _ string, _ func(string)) (watch.ShellResult, error) {
		<-ctx.Done()
		return watch.ShellResult{Canceled: true}, nil
	}
}

func TestStartTellsTheModelWhatItJustBuilt(t *testing.T) {
	call := newTool(t, blockingShell())

	out, err := call(`{"action":"start","command":"tail -f app.log","label":"errors","match":"ERROR"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "w1")
	assert.Contains(t, out, "Every matching line is an event")
	assert.Contains(t, out, "do not call this tool to check on it")

	out, err = call(`{"action":"start","command":"gh pr checks","label":"ci","every":"5m"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "runs every 5m")
	assert.Contains(t, out, "baseline")

	out, err = call(`{"action":"start","label":"check the deploy","every":"30m","once":true}`)
	require.NoError(t, err)
	assert.Contains(t, out, "comes back with its label")
}

func TestListAndStop(t *testing.T) {
	call := newTool(t, blockingShell())
	_, err := call(`{"action":"start","command":"tail -f a.log","label":"a"}`)
	require.NoError(t, err)

	out, err := call(`{"action":"list"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "w1 a")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "tail -f a.log")

	out, err = call(`{"action":"stop","id":"w1"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "Stopped w1")

	require.Eventually(t, func() bool {
		out, err := call(`{"action":"list"}`)
		return err == nil && strings.Contains(out, "stopped")
	}, 5*time.Second, 10*time.Millisecond)
}

func TestLogReplaysWhatAWatchSaw(t *testing.T) {
	call := newTool(t, func(_ context.Context, _ string, onChunk func(string)) (watch.ShellResult, error) {
		onChunk("ERROR one\nERROR two\n")
		return watch.ShellResult{}, nil
	})
	_, err := call(`{"action":"start","command":"cat log","label":"errors","match":"ERROR"}`)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		out, err := call(`{"action":"log","id":"w1"}`)
		return err == nil && strings.Contains(out, "ERROR two")
	}, 5*time.Second, 10*time.Millisecond)
}

func TestBadArgumentsExplainThemselves(t *testing.T) {
	call := newTool(t, blockingShell())

	cases := map[string]string{
		"unknown action":     `{"action":"poll"}`,
		"stop without id":    `{"action":"stop"}`,
		"log without id":     `{"action":"log"}`,
		"unparsable every":   `{"action":"start","command":"date","every":"soonish"}`,
		"interval too small": `{"action":"start","command":"date","every":"1s"}`,
		"nothing to watch":   `{"action":"start"}`,
		"unknown field":      `{"action":"list","color":"red"}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := call(args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "watch:", "the model is told which tool refused")
		})
	}
}

// TestPlanStepIsAccepted pins the plan gate's injected argument: strict
// decoding must not reject a call the gate itself rewrote.
func TestPlanStepIsAccepted(t *testing.T) {
	call := newTool(t, blockingShell())
	_, err := call(`{"action":"start","command":"tail -f a.log","label":"a","plan_step":2}`)
	assert.NoError(t, err)
}

// TestNoManagerSaysSo pins what a sub-agent gets: the tool is not registered
// for it, but a handler bound to no manager must still fail in words rather
// than panic.
func TestNoManagerSaysSo(t *testing.T) {
	tool := watchtool.Tool(watchtool.Deps{})
	_, err := tool.Run(t.Context(), json.RawMessage(`{"action":"list"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs no watches")
}

func TestDetailCarriesTheIntervalForTheApprovalPrompt(t *testing.T) {
	tool := watchtool.Tool(watchtool.Deps{})
	assert.Equal(t, "every 5m: gh pr checks",
		tool.DetailFromArgs(json.RawMessage(`{"action":"start","command":"gh pr checks","every":"5m"}`)))
	assert.Equal(t, "tail -f app.log",
		tool.DetailFromArgs(json.RawMessage(`{"command":"tail -f app.log"}`)))
	assert.Equal(t, "stop w2", tool.DetailFromArgs(json.RawMessage(`{"action":"stop","id":"w2"}`)))
}
