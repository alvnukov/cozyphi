package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// Observing hooks is not the tool loop's own pre/post hooks. The harness view
// refuses to run a hook it is describing; the executor still runs the ones a
// tool call meets, and the two must not be confused for one another.
func TestObservingHooksLeavesTheExecutorsOwnPreAndPostHooksRunning(t *testing.T) {
	var pre, post atomic.Int32
	mgr := hooks.NewManager(
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "guard",
			Pre: func(context.Context, hooks.Event) (hooks.PreResult, error) {
				pre.Add(1)
				return hooks.PreResult{Action: hooks.ActionAllow}, nil
			},
		}, Kind: hooks.KindPreTool, FailClosed: true},
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "audit",
			Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
				post.Add(1)
				return hooks.PostResult{Context: "audited"}, nil
			},
		}, Kind: hooks.KindPostTool},
	)

	// Three observations before the call, and three after: neither the view
	// nor its repetition is what decides whether a hook fires.
	for range 3 {
		require.Len(t, hooks.Observe(mgr, hooks.LoadFacts{}).Hooks, 2)
	}
	assert.Equal(t, int32(0), pre.Load(), "an observation is not a tool call")
	assert.Equal(t, int32(0), post.Load())

	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok", Output: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, mgr)
	msgs, _, stop := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"ls"}`},
	}}, func(session.ToolData) bool { return true })

	require.False(t, stop.stopped)
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), ran.Load())
	assert.Equal(t, int32(1), pre.Load(), "the tool loop's pre hook still runs")
	assert.Equal(t, int32(1), post.Load(), "and so does its post hook")
	assert.Contains(t, msgs[0].Content, "audited", "with its context still reaching the model")

	for range 3 {
		require.Len(t, hooks.Observe(mgr, hooks.LoadFacts{}).Hooks, 2)
	}
	assert.Equal(t, int32(1), pre.Load(), "and looking again afterwards fires nothing either")
	assert.Equal(t, int32(1), post.Load())
}

// A readonly turn already narrows to the fail-closed hooks. Observing the
// manager must not widen that back, and must not narrow it further: the
// executor's filter is the executor's, and the view has no say in it.
func TestObservingHooksDoesNotChangeWhatAReadonlyTurnRuns(t *testing.T) {
	var audit, strict atomic.Int32
	mgr := hooks.NewManager(
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "audit",
			Pre: func(context.Context, hooks.Event) (hooks.PreResult, error) {
				audit.Add(1)
				return hooks.PreResult{Action: hooks.ActionAllow}, nil
			},
		}, Kind: hooks.KindPreTool},
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "strict",
			MatchFn:  hooks.MatchTool("bash"),
			Pre: func(context.Context, hooks.Event) (hooks.PreResult, error) {
				strict.Add(1)
				return hooks.PreResult{Action: hooks.ActionDeny, Reason: "strict"}, nil
			},
		}, Kind: hooks.KindPreTool, FailClosed: true},
	)

	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeReadonly
	gate, err := permission.NewGate(policy, t.TempDir())
	require.NoError(t, err)

	state := hooks.Observe(mgr, hooks.LoadFacts{})
	require.Len(t, state.Hooks, 2, "the view describes both, whatever this turn would run")

	ex := NewExecutor(tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok"}, nil
			},
		},
	}, gate, nil, mgr)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"ls"}`},
	}}, func(session.ToolData) bool { return true })

	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "strict", "the fail-closed hook still denies the call")
	assert.Equal(t, int32(1), strict.Load())
	assert.Equal(t, int32(0), audit.Load(), "and the one readonly was already skipping is still skipped")
}
