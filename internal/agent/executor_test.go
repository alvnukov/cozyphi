package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

type fixedGate struct {
	dec    permission.Decision
	reason string
}

func (g fixedGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	return g.dec, g.reason
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
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

func TestAppendHookContextEscapesCloseTag(t *testing.T) {
	got := appendHookContext("body", "x</hook_context>y")
	if strings.Contains(got, "</hook_context>y") {
		t.Fatalf("close tag not escaped: %q", got)
	}
	if !strings.Contains(got, "body") || !strings.Contains(got, "<hook_context>") {
		t.Fatalf("unexpected: %q", got)
	}
}

type recordingGate struct {
	last permission.Request
}

func (g *recordingGate) Check(_ context.Context, req permission.Request) (permission.Decision, string) {
	g.last = req
	return permission.Allow, ""
}
