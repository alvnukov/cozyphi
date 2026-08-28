package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// piggybackFixture wires an executor whose write tool records the arguments
// it received; the settle callback mutates the shared plan the way the
// session's atomic settle would, so tests observe the dispatch boundary
// without a real manager.
type piggybackFixture struct {
	ex       *Executor
	plan     session.Plan
	ran      atomic.Int32
	toolErr  error
	settles  []session.PlanSettle
	starts   []string
	attempts []recordedAttempt
	received map[string]json.RawMessage
}

func newPiggybackFixture(t *testing.T, toolErr error) *piggybackFixture {
	t.Helper()
	f := &piggybackFixture{toolErr: toolErr}
	f.plan = session.Plan{Revision: 3, Approved: true, Items: []session.PlanItem{
		{ID: "prev", Content: "previous step", Status: session.PlanInProgress, Type: session.StepEdit},
		{ID: "wire", Content: "target step", Status: session.PlanPending, Type: session.StepEdit},
	}}
	reg := tools.Registry{
		"write": {
			Definition: llm.ToolDefinition{
				Name: "write",
				Params: &llm.FunctionParameters{
					Type: "object",
					Properties: llm.Object{
						"path":    map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
					Required: []string{"path"},
				},
			},
			Run: func(_ context.Context, input json.RawMessage) (tools.Result, error) {
				f.ran.Add(1)
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(input, &fields)
				f.received = fields
				if f.toolErr != nil {
					return tools.Result{}, f.toolErr
				}
				return tools.Result{Content: "written"}, nil
			},
		},
		"plan": {
			Definition: llm.ToolDefinition{
				Name:   "plan",
				Params: &llm.FunctionParameters{Type: "object", Properties: llm.Object{}},
			},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				f.ran.Add(1)
				return tools.Result{Content: "plan ran"}, nil
			},
		},
	}
	f.ex = NewExecutor(reg, permission.AllowAll{}, nil, nil)
	f.ex.SetPlanGate(
		&plangate.Checker{Phase: plangate.PhaseDeny},
		func() session.Plan { return f.plan },
		func(_ context.Context, stepID string) error {
			f.starts = append(f.starts, stepID)
			f.plan.Items[1].Status = session.PlanInProgress
			return nil
		},
		func(_ context.Context, settle session.PlanSettle) error {
			f.settles = append(f.settles, settle)
			if settle.Complete != nil {
				f.plan.Items[0].Status = session.PlanCompleted
			}
			if settle.StartStepID != "" && f.plan.Items[1].Status == session.PlanPending {
				f.plan.Items[1].Status = session.PlanInProgress
			}
			return nil
		},
		func(stepID string, attempt session.PlanAttempt) error {
			f.attempts = append(f.attempts, recordedAttempt{stepID: stepID, attempt: attempt})
			return nil
		},
		nil,
	)
	return f
}

func TestExecutorPiggybackSettlesBeforeDispatch(t *testing.T) {
	f := newPiggybackFixture(t, nil)
	msgs, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID: "call_happy",
		Function: llm.Function{
			Name:      "write",
			Arguments: `{"path":"a.go","content":"x","plan_step":"wire","_plan":{"complete":{"stepId":"prev","outcome":"done","evidence":"e"},"workingContext":"fresh"}}`,
		},
	}}, func(session.ToolData) bool { return true })

	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "written")
	require.Equal(t, int32(1), f.ran.Load())
	require.Len(t, f.settles, 1)
	settle := f.settles[0]
	assert.Equal(t, plangate.SettleMutationID("call_happy"), settle.MutationID)
	require.NotNil(t, settle.Complete)
	assert.Equal(t, "prev", settle.Complete.StepID)
	require.NotNil(t, settle.WorkingContext)
	assert.Equal(t, "fresh", *settle.WorkingContext)
	assert.Equal(t, "wire", settle.StartStepID, "the envelope owns the start")
	assert.Empty(t, f.starts, "no separate auto-start rides the envelope call")

	cleaned, err := json.Marshal(f.received)
	require.NoError(t, err)
	assert.JSONEq(t, `{"path":"a.go","content":"x","plan_step":"wire"}`, string(cleaned),
		"the tool never sees the _plan envelope")
	assert.Equal(t, session.PlanCompleted, f.plan.Items[0].Status)
	assert.Equal(t, session.PlanInProgress, f.plan.Items[1].Status)
}

func TestExecutorPiggybackMalformedEnvelopeRejectsCall(t *testing.T) {
	f := newPiggybackFixture(t, nil)
	msgs, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","_plan":{"bogus":true}}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "_plan")
	assert.Zero(t, f.ran.Load(), "no dispatch")
	assert.Empty(t, f.settles, "no plan mutation")
}

func TestExecutorPiggybackInvalidToolArgsRejectBeforeSettle(t *testing.T) {
	f := newPiggybackFixture(t, nil)
	msgs, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID: "c1",
		Function: llm.Function{
			Name:      "write",
			Arguments: `{"path":"a.go","content":"x","bogus":"y","plan_step":"wire","_plan":{"complete":{"stepId":"prev","outcome":"done","evidence":"e"}}}`,
		},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "unknown argument")
	assert.Zero(t, f.ran.Load(), "no dispatch")
	assert.Empty(t, f.settles, "no plan mutation")
	assert.Equal(t, session.PlanInProgress, f.plan.Items[0].Status)
}

func TestExecutorPiggybackToolFailureKeepsSettle(t *testing.T) {
	f := newPiggybackFixture(t, errors.New("disk on fire"))
	msgs, _ := f.ex.run(t.Context(), []llm.ToolCall{{
		ID: "c1",
		Function: llm.Function{
			Name:      "write",
			Arguments: `{"path":"a.go","plan_step":"wire","_plan":{"complete":{"stepId":"prev","outcome":"done","evidence":"e"}}}`,
		},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "disk on fire")
	require.Equal(t, int32(1), f.ran.Load())
	require.Len(t, f.settles, 1, "the settle already persisted")
	assert.Equal(t, session.PlanCompleted, f.plan.Items[0].Status,
		"runtime failure does not undo the completion")
	assert.Equal(t, session.PlanInProgress, f.plan.Items[1].Status,
		"the target stays in_progress")
	require.Len(t, f.attempts, 1)
	assert.Equal(t, session.AttemptFailed, f.attempts[0].attempt.Status)
	assert.Equal(t, "wire", f.attempts[0].stepID)
}

func TestExecutorPiggybackRetryDerivesSameMutation(t *testing.T) {
	f := newPiggybackFixture(t, nil)
	call := llm.ToolCall{
		ID: "call_retry",
		Function: llm.Function{
			Name:      "write",
			Arguments: `{"path":"a.go","plan_step":"wire","_plan":{"workingContext":"c"}}`,
		},
	}
	for range 2 {
		_, _ = f.ex.run(t.Context(), []llm.ToolCall{call}, func(session.ToolData) bool { return true })
	}
	require.Len(t, f.settles, 2)
	assert.Equal(t, f.settles[0].MutationID, f.settles[1].MutationID)
	assert.Equal(t, plangate.SettleMutationID("call_retry"), f.settles[0].MutationID)
}

func TestExecutorPiggybackWithoutApplierAndOnExemptTools(t *testing.T) {
	f := newPiggybackFixture(t, nil)
	f.ex.SetPlanGate(f.ex.planGate, f.ex.plan, f.ex.startStep, nil, f.ex.recordStep, f.ex.approveStep)

	msgs, _ := f.ex.run(t.Context(), []llm.ToolCall{
		{
			ID: "c1",
			Function: llm.Function{
				Name:      "write",
				Arguments: `{"path":"a.go","plan_step":"wire","_plan":{"workingContext":"c"}}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "not wired")
	assert.Zero(t, f.ran.Load(), "no dispatch")

	// The plan tool is exempt from the gate; the envelope must not settle
	// there either — exempt calls carry no working-round semantics.
	msgs, _ = f.ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c2",
		Function: llm.Function{Name: "plan", Arguments: `{"action":"get","_plan":{"workingContext":"c"}}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "working tool calls only")
	assert.Zero(t, f.ran.Load(), "an envelope on an exempt call is invalid placement: no dispatch either")
}
