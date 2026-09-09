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

// validateFixture registers one schema-bearing tool and one schema-less tool
// and records what reached each Run.
type validateFixture struct {
	ex       *Executor
	ran      atomic.Int32
	received json.RawMessage
	statuses []session.ToolStatus
}

func newValidateFixture(t *testing.T, gate permission.Gate, mgr *hooks.Manager) *validateFixture {
	t.Helper()
	f := &validateFixture{}
	record := func(_ context.Context, input json.RawMessage) (tools.Result, error) {
		f.ran.Add(1)
		f.received = input
		return tools.Result{Content: "ok"}, nil
	}
	reg := tools.Registry{
		"edit": {
			Definition: llm.ToolDefinition{
				Name: "edit",
				Params: &llm.FunctionParameters{
					Type: "object",
					Properties: llm.Object{
						"path":  map[string]any{"type": "string"},
						"edits": map[string]any{"type": "array"},
						"plan_step": map[string]any{
							"type": "string",
						},
					},
					Required: []string{"path", "edits", "plan_step"},
				},
			},
			Run: record,
		},
		"loose": {
			Definition: llm.ToolDefinition{Name: "loose"},
			Run:        record,
		},
	}
	f.ex = NewExecutor(reg, gate, nil, mgr)
	return f
}

func (f *validateFixture) run(t *testing.T, name, args string) llm.Message {
	t.Helper()
	msgs, _, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: name, Arguments: args},
	}}, func(d session.ToolData) bool {
		f.statuses = append(f.statuses, d.Run.Status)
		return true
	})
	require.Len(t, msgs, 1)
	return msgs[0]
}

func TestExecutorRefusesUnknownArgumentBeforeDispatch(t *testing.T) {
	f := newValidateFixture(t, permission.AllowAll{}, nil)
	msg := f.run(t, "edit", `{"path":"a.go","edits":[],"bogus":1}`)
	assert.Equal(t, `edit: invalid arguments: unknown argument "bogus"; declared: edits, path, plan_step`, msg.Content)
	assert.Zero(t, f.ran.Load(), "no dispatch")
	assert.Equal(t, []session.ToolStatus{session.ToolInProgress, session.ToolRejected}, f.statuses)
}

func TestExecutorRefusesMistypedArgumentBeforeThePermissionGate(t *testing.T) {
	gate := &recordingGate{}
	f := newValidateFixture(t, gate, nil)
	msg := f.run(t, "edit", `{"path":"a.go","edits":"not a list"}`)
	assert.Contains(t, msg.Content, `argument "edits" must be array, not string`)
	assert.Empty(t, gate.last.Tool, "a doomed call never reaches the permission gate")
	assert.Zero(t, f.ran.Load(), "no dispatch")
}

func TestExecutorLeavesMissingPlanStepToThePlanGate(t *testing.T) {
	f := newValidateFixture(t, permission.AllowAll{}, nil)
	msg := f.run(t, "edit", `{"path":"a.go","edits":[]}`)
	assert.Equal(t, "ok", msg.Content, "plan_step is required by the schema, but its absence is the gate's call")
	assert.Equal(t, int32(1), f.ran.Load())
}

func TestExecutorAcceptsFilePathAliasForPath(t *testing.T) {
	f := newValidateFixture(t, permission.AllowAll{}, nil)
	msg := f.run(t, "edit", `{"file_path":"a.go","edits":[],"plan_step":"wire"}`)
	assert.Equal(t, "ok", msg.Content)
	assert.JSONEq(t, `{"file_path":"a.go","edits":[],"plan_step":"wire"}`, string(f.received),
		"the alias reaches the tool untouched; edit resolves it itself")
}

func TestExecutorLeavesSchemalessToolToItsDecoder(t *testing.T) {
	f := newValidateFixture(t, permission.AllowAll{}, nil)
	msg := f.run(t, "loose", `{"anything":"goes"}`)
	assert.Equal(t, "ok", msg.Content)
	assert.Equal(t, int32(1), f.ran.Load())
	msg = f.run(t, "loose", `[1,2]`)
	assert.Contains(t, msg.Content, "arguments must be one JSON object")
	assert.Equal(t, int32(1), f.ran.Load(), "the one-object shape still holds")
}

func TestExecutorValidatesHookRewrittenArguments(t *testing.T) {
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "rewrite",
			MatchFn:  hooks.MatchTool("edit"),
			Pre: func(_ context.Context, _ hooks.Event) (hooks.PreResult, error) {
				return hooks.PreResult{
					Action: hooks.ActionModify,
					Input:  json.RawMessage(`{"path":"a.go","edits":[],"injected":true}`),
				}, nil
			},
		},
		Kind: hooks.KindPreTool,
	})
	f := newValidateFixture(t, permission.AllowAll{}, mgr)
	msg := f.run(t, "edit", `{"path":"a.go","edits":[],"plan_step":"wire"}`)
	assert.Contains(t, msg.Content, `unknown argument "injected"`)
	assert.Zero(t, f.ran.Load(), "a hook cannot smuggle an undeclared argument past the schema")
}
