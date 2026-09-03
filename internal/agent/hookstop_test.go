package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// A hook that stops without saying why is still a stop: the zero value means
// "no hook asked", which an empty reason cannot say on its own.
func TestHookStopDistinguishesSilenceFromAbsence(t *testing.T) {
	var none hookStop
	assert.NoError(t, none.Err())
	assert.Empty(t, none.Reason())

	silent := hookStop{stopped: true}
	require.ErrorIs(t, silent.Err(), ErrPostHookStop)
	assert.Equal(t, defaultHookStopReason, silent.Reason(), "the model is told something")
	assert.Equal(t, ErrPostHookStop, silent.Err(), "no reason to wrap, so nothing is added")

	spoken := hookStop{stopped: true, reason: "audit trip"}
	require.ErrorIs(t, spoken.Err(), ErrPostHookStop)
	assert.Contains(t, spoken.Err().Error(), "audit trip")
	assert.Equal(t, "audit trip", spoken.Reason())
}

// A hook that stops silently still explains itself to the model, using the
// one default the error path reads too.
func TestExecutorSilentStopReachesTheModelAndTheEngine(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ran", Output: "ran"}, nil
			},
		},
	}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "quiet",
			Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
				return hooks.PostResult{Stop: true}, nil
			},
		},
		Kind: hooks.KindPostTool,
	})
	ex := NewExecutor(reg, permission.AllowAll{}, nil, mgr)

	msgs, _, stop := ex.run(t.Context(), []llm.ToolCall{
		{ID: "c1", Function: llm.Function{Name: "bash", Arguments: `{"command":"echo one"}`}},
	}, func(session.ToolData) bool { return true })

	require.True(t, stop.stopped)
	assert.ErrorIs(t, stop.Err(), ErrPostHookStop)
	assert.Contains(t, msgs[0].Content, defaultHookStopReason,
		"the reason the model reads is the one the error uses")
}

// Two hooks stopping the same call arrive as one aggregated reason, and it
// travels whole: the model reads the same sentence the run's error carries.
func TestExecutorCarriesEveryStoppingHooksReason(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ran", Output: "ran"}, nil
			},
		},
	}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "first",
			Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
				return hooks.PostResult{Stop: true, Reason: "first reason"}, nil
			},
		},
		Kind: hooks.KindPostTool,
	}, hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "second",
			Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
				return hooks.PostResult{Stop: true, Reason: "second reason"}, nil
			},
		},
		Kind: hooks.KindPostTool,
	})
	ex := NewExecutor(reg, permission.AllowAll{}, nil, mgr)

	msgs, _, stop := ex.run(t.Context(), []llm.ToolCall{
		{ID: "c1", Function: llm.Function{Name: "bash", Arguments: `{"command":"echo one"}`}},
	}, func(session.ToolData) bool { return true })

	assert.Contains(t, stop.Reason(), "first reason")
	assert.Contains(t, stop.Reason(), "second reason")
	assert.Contains(t, stop.Err().Error(), stop.Reason())
	assert.Contains(t, msgs[0].Content, stop.Reason())
}
