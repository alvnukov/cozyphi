package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

type fixedGate struct {
	dec    permission.Decision
	reason string
}

func (g fixedGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	return g.dec, g.reason
}

func TestExecutorConsumerStopReturnsCancellationForEveryToolCall(t *testing.T) {
	var runs atomic.Int32
	ex := NewExecutor(tools.Registry{
		"count": countingTool(&runs),
	}, permission.AllowAll{}, nil, nil)
	calls := []llm.ToolCall{
		{ID: "call_1", Function: llm.Function{Name: "count", Arguments: `{}`}},
		{ID: "call_2", Function: llm.Function{Name: "count", Arguments: `{}`}},
		{ID: "call_3", Function: llm.Function{Name: "count", Arguments: `{}`}},
	}

	results, active, _ := ex.run(t.Context(), calls, func(session.ToolData) bool { return false })

	require.False(t, active)
	require.Zero(t, runs.Load())
	require.Len(t, results, len(calls))
	for i, result := range results {
		require.Equal(t, llm.RoleTool, result.Role)
		require.Equal(t, calls[i].ID, result.ToolCallID)
		require.Equal(t, ToolCanceledResult, result.Content)
	}
}

func TestExecutorDenyDoesNotRunHandler(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Deny, reason: "denied by test"}, nil, nil)
	var statuses []session.ToolStatus
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"echo hi"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})
	if ran.Load() != 0 {
		t.Fatal("handler should not run on deny")
	}
	if len(msgs) != 1 || msgs[0].Content != "denied by test" {
		t.Fatalf("tool message: %+v", msgs)
	}
	found := false
	for _, s := range statuses {
		if s == session.ToolRejected {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ToolRejected in %v", statuses)
	}
}

func TestExecutorAskFalseRejects(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ask := func(context.Context, permission.Request, string) (permission.AskResult, error) {
		return permission.AskResult{Approved: false}, nil
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask, nil)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"curl x"}`},
	}}, func(session.ToolData) bool { return true })
	if ran.Load() != 0 {
		t.Fatal("handler should not run when ask denied")
	}
	if len(msgs) != 1 || msgs[0].Content == "" {
		t.Fatalf("expected rejection message, got %+v", msgs)
	}
}

func TestExecutorEmitsToolName(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	var names []string
	_, _, _ = ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd"}`},
	}}, func(td session.ToolData) bool {
		names = append(names, td.Run.Name)
		return true
	})
	if len(names) == 0 {
		t.Fatal("expected tool events")
	}
	for _, n := range names {
		if n != "bash" {
			t.Fatalf("expected Name=bash on every ToolData, got %q in %v", n, names)
		}
	}
}

func TestExecutorAskNilRejectsHeadless(t *testing.T) {
	// Headless mode wires Ask=nil: an Ask decision must reject without
	// running the handler (Ask≡Deny), even if the gate did not fold it.
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, nil, nil)
	var statuses []session.ToolStatus
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /tmp/x"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})
	if ran.Load() != 0 {
		t.Fatal("handler should not run when ask handler is nil (headless)")
	}
	if len(msgs) != 1 || msgs[0].Content == "" {
		t.Fatalf("expected rejection message, got %+v", msgs)
	}
	found := false
	for _, s := range statuses {
		if s == session.ToolRejected {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ToolRejected in %v", statuses)
	}
}

func TestExecutorAskTrueRuns(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ran"}, nil
			},
		},
	}
	ask := func(context.Context, permission.Request, string) (permission.AskResult, error) {
		return permission.AskResult{Approved: true}, nil
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask, nil)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"curl x"}`},
	}}, func(session.ToolData) bool { return true })
	if ran.Load() != 1 {
		t.Fatal("handler should run when ask approved")
	}
	if len(msgs) != 1 || msgs[0].Content != "ran" {
		t.Fatalf("got %+v", msgs)
	}
}

func TestExecutorAskFeedbackMessage(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ask := func(context.Context, permission.Request, string) (permission.AskResult, error) {
		return permission.AskResult{Approved: false, Feedback: "use go test instead"}, nil
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "ask me"}, ask, nil)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"curl x"}`},
	}}, func(session.ToolData) bool { return true })
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "use go test instead") {
		t.Fatalf("expected feedback in message, got %+v", msgs)
	}
}

func TestExecutorNilAskOnAskDenies(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "ask me"}, nil, nil)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"curl x"}`},
	}}, func(session.ToolData) bool { return true })
	if len(msgs) != 1 || msgs[0].Content == "" {
		t.Fatalf("expected deny message, got %+v", msgs)
	}
}

func TestExecutorHookDenySkipsGateAsk(t *testing.T) {
	var ran atomic.Int32
	var askCalled atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ask := func(context.Context, permission.Request, string) (permission.AskResult, error) {
		askCalled.Add(1)
		return permission.AskResult{Approved: true}, nil
	}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "guard",
			MatchFn:  hooks.MatchTool("bash"),
			Pre: func(_ context.Context, _ hooks.Event) (hooks.PreResult, error) {
				return hooks.PreResult{Action: hooks.ActionDeny, Reason: "hook blocked"}, nil
			},
		},
		Kind: hooks.KindPreTool,
	})
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask, mgr)
	var statuses []session.ToolStatus
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})
	if ran.Load() != 0 {
		t.Fatal("handler must not run when hook denies")
	}
	if askCalled.Load() != 0 {
		t.Fatal("gate Ask must not run when hook denies")
	}
	if len(msgs) != 1 || msgs[0].Content != "hook blocked" {
		t.Fatalf("tool message: %+v", msgs)
	}
	found := false
	for _, s := range statuses {
		if s == session.ToolRejected {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ToolRejected in %v", statuses)
	}
}

func TestExecutorHookModifySeenByGateAndRun(t *testing.T) {
	var sawArgs string
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			DetailFromArgs: func(input json.RawMessage) string {
				var in struct {
					Command string `json:"command"`
				}
				_ = json.Unmarshal(input, &in)
				return in.Command
			},
			Run: func(_ context.Context, input json.RawMessage) (tools.Result, error) {
				sawArgs = string(input)
				return tools.Result{Content: "ran", Output: "ran"}, nil
			},
		},
	}
	gate := &recordingGate{}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "rewrite",
			MatchFn:  hooks.MatchTool("bash"),
			Pre: func(_ context.Context, _ hooks.Event) (hooks.PreResult, error) {
				return hooks.PreResult{
					Action: hooks.ActionModify,
					Input:  json.RawMessage(`{"command":"echo safe"}`),
				}, nil
			},
		},
		Kind: hooks.KindPreTool,
	})
	ex := NewExecutor(reg, gate, nil, mgr)
	var detail string
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"rm -rf /"}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolDone {
			detail = td.Run.Detail
		}
		return true
	})
	if gate.last.Command != "echo safe" {
		t.Fatalf("gate saw command %q, want modified", gate.last.Command)
	}
	if !strings.Contains(sawArgs, "echo safe") {
		t.Fatalf("handler saw %q", sawArgs)
	}
	if detail != "echo safe" {
		t.Fatalf("UI detail %q", detail)
	}
	if len(msgs) != 1 || msgs[0].Content != "ran" {
		t.Fatalf("got %+v", msgs)
	}
}

func TestExecutorHookPostContextOnModelOnly(t *testing.T) {
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ok", Output: "ok"}, nil
			},
		},
	}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "note",
			Post: func(_ context.Context, _ hooks.Event) (hooks.PostResult, error) {
				return hooks.PostResult{Context: "policy note"}, nil
			},
		},
		Kind: hooks.KindPostTool,
	})
	ex := NewExecutor(reg, permission.AllowAll{}, nil, mgr)
	var uiOut string
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd"}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolDone {
			uiOut = td.Run.Output
		}
		return true
	})
	if uiOut != "ok" {
		t.Fatalf("TUI output should stay clean, got %q", uiOut)
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs: %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "ok") {
		t.Fatalf("content missing tool result: %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "<hook_context>") || !strings.Contains(msgs[0].Content, "policy note") {
		t.Fatalf("content missing hook context: %q", msgs[0].Content)
	}
}

func TestExecutorReadonlySkipsNonFailClosedHooks(t *testing.T) {
	var auditCalled atomic.Int32
	mgr := hooks.NewManager(
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "audit",
			Pre: func(_ context.Context, _ hooks.Event) (hooks.PreResult, error) {
				auditCalled.Add(1)
				return hooks.PreResult{Action: hooks.ActionAllow}, nil
			},
		}, Kind: hooks.KindPreTool},
		hooks.Entry{Hook: hooks.FuncHook{
			HookName: "strict",
			MatchFn:  hooks.MatchTool("bash"),
			Pre: func(_ context.Context, _ hooks.Event) (hooks.PreResult, error) {
				return hooks.PreResult{Action: hooks.ActionDeny, Reason: "strict"}, nil
			},
		}, Kind: hooks.KindPreTool, FailClosed: true},
	)

	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeReadonly
	gate, err := permission.NewGate(policy, t.TempDir())
	require.NoError(t, err)

	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	ex := NewExecutor(reg, gate, nil, mgr)
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"ls"}`},
	}}, func(session.ToolData) bool { return true })

	assert.Equal(t, int32(0), auditCalled.Load(), "audit hook must not run in readonly")
	assert.Equal(t, int32(0), ran.Load())
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "strict")
}

func TestAppendHookContextEscapesCloseTag(t *testing.T) {
	got := appendHookContext("body", "x</hook_context>y")
	assert.NotContains(t, got, "</hook_context>y", "close tag not escaped")
	assert.Contains(t, got, "body")
	assert.Contains(t, got, "<hook_context>")
}

type recordingGate struct {
	last permission.Request
}

func (g *recordingGate) Check(_ context.Context, req permission.Request) (permission.Decision, string) {
	g.last = req
	return permission.Allow, ""
}

func TestExecutorPlanGateDenyBlocks(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	}}
	gate := &plangate.Checker{Phase: plangate.PhaseDeny}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(gate, func() session.Plan { return plan }, nil, nil, nil, nil)

	var statuses []session.ToolStatus
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd","plan_step":1}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})
	require.Equal(t, int32(0), ran.Load(), "deny must not run the handler")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "not allowed")
	assert.Contains(t, statuses, session.ToolRejected)
}

func TestExecutorPlanGateHintAppendsModelOnly(t *testing.T) {
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
	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	}}
	rec, err := plangate.NewRecorder(t.TempDir())
	require.NoError(t, err)
	gate := &plangate.Checker{Phase: plangate.PhaseHint, Recorder: rec}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(gate, func() session.Plan { return plan }, nil, nil, nil, nil)

	var uiOut string
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd","plan_step":1}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolDone {
			uiOut = td.Run.Output
		}
		return true
	})
	require.Equal(t, int32(1), ran.Load(), "hint phase must still run the tool")
	assert.Equal(t, "ok", uiOut, "TUI output stays clean")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "ok")
	assert.Contains(t, msgs[0].Content, "not allowed", "hint reaches the model only")
}

func TestExecutorPlanGateUnapprovedDeniesInDenyPhase(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
	}
	gate := &plangate.Checker{Phase: plangate.PhaseDeny}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(gate, func() session.Plan { return session.Plan{Approved: false} }, nil, nil, nil, nil)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd"}`},
	}}, func(session.ToolData) bool { return true })
	require.Equal(t, int32(0), ran.Load(), "unapproved plan must block the tool in deny phase")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "not approved")
}

type denyGate struct{}

func (denyGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	return permission.Deny, "locked down"
}

// recordedAttempt pairs one filed attempt with the step it landed on.
type recordedAttempt struct {
	stepID  string
	attempt session.PlanAttempt
}

// autoStartFixture wires an executor against a plan the test can flip: the
// start callback mirrors what a real session transition does to the snapshot,
// and the recorder collects the attempt evidence the executor files.
func autoStartFixture(t *testing.T, stepStatus session.PlanStatus) (
	*Executor,
	func() session.PlanItem, // read the step back
	func(error), // install the start outcome
	func(error), // install the tool outcome
	*atomic.Int32, // tool runs
	*[]recordedAttempt, // filed attempt evidence
) {
	t.Helper()
	var ran atomic.Int32
	var toolErr error
	var recorded []recordedAttempt
	reg := tools.Registry{
		"write": {
			Definition: llm.ToolDefinition{Name: "write"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				if toolErr != nil {
					return tools.Result{}, toolErr
				}
				return tools.Result{Content: "written"}, nil
			},
		},
		// bash exists only so a wrong-type call reaches the plan gate instead
		// of dying as an unknown tool.
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ran"}, nil
			},
		},
	}
	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{{
		ID: "wire", Content: "write it", Status: stepStatus, Type: session.StepEdit,
	}}}
	var startErr error
	gate := &plangate.Checker{Phase: plangate.PhaseDeny}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(gate, func() session.Plan { return plan }, func(_ context.Context, stepID string) error {
		if stepID != "wire" {
			return fmt.Errorf("unexpected step %q", stepID)
		}
		if err := startErr; err != nil {
			return err
		}
		plan.Items[0].Status = session.PlanInProgress
		return nil
	}, nil, func(stepID string, attempt session.PlanAttempt) error {
		recorded = append(recorded, recordedAttempt{stepID: stepID, attempt: attempt})
		return nil
	}, nil)
	step := func() session.PlanItem {
		for _, item := range plan.Items {
			if item.ID == "wire" {
				return item
			}
		}
		return session.PlanItem{}
	}
	return ex, step, func(err error) { startErr = err }, func(err error) { toolErr = err }, &ran, &recorded
}

func TestExecutorAutoStartsPendingStepBeforeDispatch(t *testing.T) {
	ex, step, fail, _, ran, _ := autoStartFixture(t, session.PlanPending)
	fail(nil)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Equal(t, int32(1), ran.Load(), "the tool must run without a separate plan call")
	assert.Contains(t, msgs[0].Content, "written")
	assert.Equal(t, session.PlanInProgress, step().Status, "the pending step is in_progress by dispatch time")
}

func TestExecutorAutoStartFailureRejectsWithoutDispatch(t *testing.T) {
	ex, step, fail, _, ran, _ := autoStartFixture(t, session.PlanPending)
	fail(errors.New("session closed"))

	var statuses []session.ToolStatus
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(td session.ToolData) bool {
		statuses = append(statuses, td.Run.Status)
		return true
	})
	require.Equal(t, int32(0), ran.Load(), "a failed start must not dispatch the tool")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, `start plan step "wire"`)
	assert.Contains(t, msgs[0].Content, "session closed")
	assert.Contains(t, statuses, session.ToolRejected)
	assert.Equal(t, session.PlanPending, step().Status)
}

func TestExecutorAutoStartLostRaceProceeds(t *testing.T) {
	ex, _, _, _, ran, _ := autoStartFixture(t, session.PlanInProgress)

	// The plan supplier reports the step in_progress (another call won the
	// race), while the transition itself still errors: the call proceeds.
	ex.SetPlanGate(
		ex.planGate,
		ex.plan,
		func(context.Context, string) error { return errors.New("step is in_progress") },
		ex.settlePlan,
		ex.recordStep,
		ex.approveStep,
	)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Equal(t, int32(1), ran.Load(), "a start that lost the race is success")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "written")
}

// TestExecutorAutoStartsPendingStepOnExemptBinding: the voluntary plan_step
// binding is the start door for read-only steps — a read that names a pending
// step starts it before dispatch, files its attempt evidence there, and the
// tool still runs, because exemption only lifts the requirement, never the
// binding.
func TestExecutorAutoStartsPendingStepOnExemptBinding(t *testing.T) {
	var ran atomic.Int32
	var recorded []recordedAttempt
	reg := tools.Registry{
		"read": {
			Definition: llm.ToolDefinition{Name: "read"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "read"}, nil
			},
		},
	}
	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{{
		ID: "probe", Content: "read it", Status: session.PlanPending, Type: session.StepExplore,
	}}}
	policy, err := plangate.Compile(plangate.Defaults{
		Types:                []plangate.TypeDefaults{{Name: session.StepExplore, Tools: []string{"lsp"}}},
		AdditionalExemptions: []string{"read"},
	})
	require.NoError(t, err)
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(
		&plangate.Checker{Phase: plangate.PhaseDeny, Policy: policy},
		func() session.Plan { return plan },
		func(_ context.Context, stepID string) error {
			require.Equal(t, "probe", stepID)
			plan.Items[0].Status = session.PlanInProgress
			return nil
		},
		nil,
		func(stepID string, attempt session.PlanAttempt) error {
			recorded = append(recorded, recordedAttempt{stepID: stepID, attempt: attempt})
			return nil
		},
		nil,
	)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "read", Arguments: `{"path":"a.go","plan_step":"probe"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "read")
	assert.Equal(t, int32(1), ran.Load(), "the exempt read runs without ceremony")
	assert.Equal(t, session.PlanInProgress, plan.Items[0].Status, "the binding started the step before dispatch")
	require.Len(t, recorded, 1)
	assert.Equal(t, "probe", recorded[0].stepID, "the call files its attempt on the step it bound")
}

func TestExecutorGateMissDoesNotStart(t *testing.T) {
	ex, step, fail, _, ran, recorded := autoStartFixture(t, session.PlanPending)
	fail(nil)

	// bash is beyond an edit step's reach: the gate refuses, so nothing starts.
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"pwd","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Equal(t, int32(0), ran.Load())
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "not allowed")
	assert.Empty(t, *recorded, "a refused call files no evidence")
	assert.Equal(t, session.PlanPending, step().Status, "a refused call moves no status")
}

func TestExecutorPermissionDenialDoesNotStart(t *testing.T) {
	ex, step, fail, _, _, recorded := autoStartFixture(t, session.PlanPending)
	fail(nil)
	ex.gate = denyGate{}
	ex.syncHookFilter()

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "locked down")
	assert.Empty(t, *recorded, "a denied call files no evidence")
	assert.Equal(t, session.PlanPending, step().Status, "a denied call starts nothing")
}

func TestExecutorRuntimeFailureKeepsStepStarted(t *testing.T) {
	ex, step, fail, failTool, _, recorded := autoStartFixture(t, session.PlanPending)
	fail(nil)
	failTool(errors.New("disk full"))

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "disk full")
	require.Len(t, *recorded, 1)
	assert.Equal(t, session.AttemptFailed, (*recorded)[0].attempt.Status)
	assert.Equal(t, "wire", (*recorded)[0].stepID)
	assert.Equal(t, session.PlanInProgress, step().Status, "a runtime failure leaves the step started and retryable")
}

func TestExecutorLegacyOrdinalNoteReachesModel(t *testing.T) {
	ex, _, fail, _, ran, _ := autoStartFixture(t, session.PlanInProgress)
	fail(nil)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":1}`},
	}}, func(session.ToolData) bool { return true })
	require.Equal(t, int32(1), ran.Load())
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "deprecated", "numeric plan_step is answered with the deprecation note")
}

func TestExecutorRecordsAttemptOnSuccess(t *testing.T) {
	ex, _, fail, _, ran, recorded := autoStartFixture(t, session.PlanInProgress)
	fail(nil)

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	require.Equal(t, int32(1), ran.Load())
	require.Len(t, *recorded, 1)
	got := (*recorded)[0]
	assert.Equal(t, "wire", got.stepID)
	assert.Equal(t, "c1", got.attempt.CallID)
	assert.Equal(t, "write", got.attempt.Tool)
	assert.Equal(t, session.AttemptSuccess, got.attempt.Status)
	assert.Equal(t, "written", got.attempt.Summary)
	assert.False(t, got.attempt.At.IsZero())
}

func TestExecutorRecordsLostAttemptWhenResultUndelivered(t *testing.T) {
	ex, _, fail, _, ran, recorded := autoStartFixture(t, session.PlanInProgress)
	fail(nil)

	// The in-progress row is delivered, the terminal one is not: the event
	// consumer died after dispatch, so the computed result is lost.
	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(td session.ToolData) bool { return td.Run.Status == session.ToolInProgress })
	require.Len(t, msgs, 1)
	require.Equal(t, int32(1), ran.Load(), "the tool itself completed")
	require.Len(t, *recorded, 1)
	assert.Equal(t, session.AttemptLost, (*recorded)[0].attempt.Status, "a result the consumer never saw is lost")
}

func TestExecutorRecordsCanceledAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	plan := session.Plan{Revision: 1, Approved: true, Items: []session.PlanItem{{
		ID: "wire", Content: "write it", Status: session.PlanInProgress, Type: session.StepEdit,
	}}}
	var recorded []recordedAttempt
	reg := tools.Registry{
		"write": {
			Definition: llm.ToolDefinition{Name: "write"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				cancel()
				return tools.Result{}, context.Canceled
			},
		},
	}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, nil)
	ex.SetPlanGate(&plangate.Checker{Phase: plangate.PhaseDeny}, func() session.Plan { return plan }, nil, nil,
		func(stepID string, attempt session.PlanAttempt) error {
			recorded = append(recorded, recordedAttempt{stepID: stepID, attempt: attempt})
			return nil
		}, nil)

	msgs, _, _ := ex.run(ctx, []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, ToolCanceledResult)
	require.Len(t, recorded, 1)
	assert.Equal(t, "wire", recorded[0].stepID)
	assert.Equal(t, session.AttemptCanceled, recorded[0].attempt.Status)
}

func TestExecutorRecordingFailureSurfacesNotice(t *testing.T) {
	ex, _, fail, _, ran, _ := autoStartFixture(t, session.PlanInProgress)
	fail(nil)
	ex.recordStep = func(string, session.PlanAttempt) error { return errors.New("budget full") }

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "write", Arguments: `{"path":"a.go","content":"x","plan_step":"wire"}`},
	}}, func(session.ToolData) bool { return true })
	require.Len(t, msgs, 1)
	require.Equal(t, int32(1), ran.Load(), "the tool result itself is untouched")
	assert.Contains(t, msgs[0].Content, "written")
	assert.Contains(t, msgs[0].Content, "attempt evidence was not recorded")
	assert.Contains(t, msgs[0].Content, "budget full")
}

// A PostTool hook's Stop must end the agentic loop with its Reason: the doc
// contract for exit-2 / stop:true is a hard stop, so the round finishes (one
// result per advertised tool call, pairing preserved), later calls in the same
// round do not run, and the reason travels up to the engine.
func TestExecutorPostHookStopEndsRoundWithReason(t *testing.T) {
	var ran atomic.Int32
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ran", Output: "ran"}, nil
			},
		},
	}
	mgr := hooks.NewManager(hooks.Entry{
		Hook: hooks.FuncHook{
			HookName: "audit",
			Post: func(_ context.Context, _ hooks.Event) (hooks.PostResult, error) {
				return hooks.PostResult{Stop: true, Reason: "audit trip"}, nil
			},
		},
		Kind: hooks.KindPostTool,
	})
	ex := NewExecutor(reg, permission.AllowAll{}, nil, mgr)

	msgs, active, stop := ex.run(t.Context(), []llm.ToolCall{
		{ID: "c1", Function: llm.Function{Name: "bash", Arguments: `{"command":"echo one"}`}},
		{ID: "c2", Function: llm.Function{Name: "bash", Arguments: `{"command":"echo two"}`}},
	}, func(session.ToolData) bool { return true })

	if stop != "audit trip" {
		t.Fatalf("stop reason: want %q, got %q", "audit trip", stop)
	}
	if !active {
		t.Fatal("the event consumer is alive; only the loop stops")
	}
	if ran.Load() != 1 {
		t.Fatalf("calls after the stop must not run, tool ran %d times", ran.Load())
	}
	if len(msgs) != 2 {
		t.Fatalf("one result per advertised call, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "stopped") || !strings.Contains(msgs[0].Content, "audit trip") {
		t.Fatalf("stopped call must tell the model why: %q", msgs[0].Content)
	}
}

// An exit-2 CommandHook wired as post_tool stops the same way: the external
// contract and the in-process one share one code path.
func TestExecutorCommandHookExit2StopsRun(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "deny.sh")
	// The hook body is empty; exit 2 alone is the stop contract.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := hooks.NewCommandHook(hooks.Discovered{
		Manifest: hooks.Manifest{Name: "guard", Kind: hooks.KindPostTool, Run: script},
		RunPath:  script,
	})
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "ran", Output: "ran"}, nil
			},
		},
	}
	ex := NewExecutor(reg, permission.AllowAll{}, nil, hooks.NewManager(hooks.Entry{Hook: h, Kind: hooks.KindPostTool}))
	_, _, stop := ex.run(t.Context(), []llm.ToolCall{
		{ID: "c1", Function: llm.Function{Name: "bash", Arguments: `{"command":"echo one"}`}},
	}, func(session.ToolData) bool { return true })
	if stop == "" {
		t.Fatal("exit-2 post hook must stop the run")
	}
	if !strings.Contains(stop, "exit 2") {
		t.Fatalf("stop reason should name exit 2, got %q", stop)
	}
}
