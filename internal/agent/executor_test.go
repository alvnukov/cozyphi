package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

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
	ex := NewExecutor(reg, fixedGate{dec: permission.Deny, reason: "denied by test"}, nil)
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
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask)
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
	ex := NewExecutor(reg, permission.AllowAll{}, nil)
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
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, nil)
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
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "needs approval"}, ask)
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
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "ask me"}, ask)
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
	ex := NewExecutor(reg, fixedGate{dec: permission.Ask, reason: "ask me"}, nil)
	msgs := ex.Run(context.Background(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"curl x"}`},
	}}, func(session.ToolData) bool { return true })
	if len(msgs) != 1 || msgs[0].Content == "" {
		t.Fatalf("expected deny message, got %+v", msgs)
	}
}
