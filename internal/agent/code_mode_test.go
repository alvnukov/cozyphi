package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func codeTestCall() llm.ToolCall {
	return llm.ToolCall{
		ID:       "cell",
		Type:     "function",
		Function: llm.Function{Name: "code", Arguments: `{"source":"return 2"}`},
	}
}

type codeTestHost = func(context.Context, string, json.RawMessage) (any, error)

func codeExecutor(
	run func(context.Context, codeTestHost) (CodeResult, error),
	extra tools.Tool,
	hook *hooks.Manager,
) *Executor {
	e := NewExecutor(tools.NewRegistry([]tools.Tool{codeTool(), extra}), permission.AllowAll{}, nil, hook)
	e.codeMode = &CodeMode{
		Run: func(ctx context.Context, _, _ string, _ bool, _ string, _ []llm.ToolDefinition, call codeTestHost) (CodeResult, error) {
			return run(ctx, call)
		},
	}
	return e
}

func TestCodeRunsThroughHooksAndPreservesControlNotes(t *testing.T) {
	var ran atomic.Int32
	tool := tools.Tool{
		Definition: llm.ToolDefinition{Name: "bash"},
		Run: func(_ context.Context, args json.RawMessage) (tools.Result, error) {
			ran.Add(1)
			require.JSONEq(t, `{"command":"safe"}`, string(args))
			return tools.Result{Content: "original", Structured: map[string]int{"internal": 1}}, nil
		},
	}
	h := hooks.NewManager(hooks.Entry{Kind: hooks.KindPreTool, Hook: hooks.FuncHook{
		HookName: "rewrite",
		MatchFn:  hooks.MatchTool("bash"),
		Pre: func(context.Context, hooks.Event) (hooks.PreResult, error) {
			return hooks.PreResult{Action: hooks.ActionModify, Input: json.RawMessage(`{"command":"safe"}`)}, nil
		},
	}}, hooks.Entry{Kind: hooks.KindPostTool, Hook: hooks.FuncHook{
		HookName: "redact",
		MatchFn:  hooks.MatchTool("bash"),
		Post: func(context.Context, hooks.Event) (hooks.PostResult, error) {
			return hooks.PostResult{
				Output:  "redacted",
				Context: "policy reminder",
				Stop:    true,
				Reason:  "stop here",
			}, nil
		},
	}})
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		value, err := call(ctx, "bash", json.RawMessage(`{"command":"before"}`))
		require.NoError(t, err)
		require.Equal(t, "redacted", value)
		_, err = call(ctx, "bash", json.RawMessage(`{}`))
		require.Error(t, err)
		return CodeResult{Output: "no nested data"}, nil
	}, tool, h)
	gate := &recordingGate{}
	e.gate = gate
	var trace []session.ToolRun
	e.saveCodeTrace = func(run session.ToolRun) error { trace = append(trace, run); return nil }
	results, _, stop := e.run(t.Context(), []llm.ToolCall{codeTestCall()}, func(session.ToolData) bool { return true })
	require.Equal(t, "stop here", stop.Reason())
	require.EqualValues(t, 1, ran.Load())
	require.Len(t, trace, 2)
	require.Equal(t, "cell", trace[0].ParentUseID)
	require.Equal(t, "redacted", trace[1].Output)
	require.Equal(t, "safe", gate.last.Command)
	require.Contains(t, results[0].Content, "policy reminder")
	require.NotContains(t, results[0].Content, "original")
}

func TestCodeNestedTypedResultAndReceipts(t *testing.T) {
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		for range 2 {
			value, err := call(ctx, "agent_wait", json.RawMessage(`{}`))
			require.NoError(t, err)
			require.Equal(t, 42, value)
		}
		return CodeResult{Output: "selected", Imports: map[string]string{"counter": "1.0.0@digest"}}, nil
	}, tools.Tool{Definition: llm.ToolDefinition{Name: "agent_wait"}, Run: func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: "child outcome", Structured: 42, DeliveryID: "job:terminal"}, nil
	}}, nil)
	messages, _, _ := e.run(t.Context(), []llm.ToolCall{codeTestCall()}, func(session.ToolData) bool { return true })
	require.Len(t, messages, 1)
	m := messages[0]
	require.Contains(t, m.Content, "child outcome")
	require.Equal(t, "1.0.0@digest", m.CodeImports["counter"])
	require.Contains(t, m.DeliveryIDs, "job:terminal")
	wire, err := json.Marshal(m)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "digest")
	require.NotContains(t, string(wire), "job:terminal")
}

func TestCodeRefusesWhenTraceCannotBeWritten(t *testing.T) {
	var ran atomic.Int32
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		_, err := call(ctx, "count", json.RawMessage(`{}`))
		require.Error(t, err)
		return CodeResult{}, nil
	}, countingTool(&ran), nil)
	e.saveCodeTrace = func(session.ToolRun) error { return errors.New("disk full") }
	results, _, _ := e.run(t.Context(), []llm.ToolCall{codeTestCall()}, func(session.ToolData) bool { return true })
	require.Zero(t, ran.Load())
	require.Contains(t, results[0].Content, "disk full")
}

func TestCodeCrashRecoveryPreservesEffectsOutsideProviderHistory(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSession(SessionOpts{Cwd: dir, SessionDir: dir, Persist: true})
	require.NoError(t, err)
	require.NoError(t, s.Append(llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{codeTestCall()}}))
	for i, status := range []session.ToolStatus{session.ToolDone, session.ToolInProgress} {
		id := []string{"a", "b"}[i]
		require.NoError(
			t,
			s.manager.AppendCodeTrace(
				session.ToolRun{ParentUseID: "cell", ToolUseID: id, Name: "write", Status: status, Output: "diff"},
			),
		)
	}
	path := s.File()
	require.NoError(t, s.Close())
	s, err = NewSession(SessionOpts{ResumePath: path})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	m := s.BuildContext()
	require.Len(t, m, 2)
	require.Contains(t, m[1].Content, "outcome unknown")
	require.Contains(t, m[1].Content, "a write:")
	require.NotContains(t, m[1].Content, "diff")
	require.True(
		t,
		slices.ContainsFunc(
			s.ReplayEntries(),
			func(e session.MessageEntry) bool { return e.GetType() == session.EntryCodeTrace },
		),
	)
}

func TestCodeYieldWaitCarriesOnlyCurrentTurnAuthority(t *testing.T) {
	ready := make(chan struct{})
	var ran atomic.Int32
	e := codeExecutor(func(ctx context.Context, call codeTestHost) (CodeResult, error) {
		select {
		case <-ready:
		case <-ctx.Done():
			return CodeResult{}, ctx.Err()
		}
		_, err := call(ctx, "count", json.RawMessage(`{}`))
		return CodeResult{Output: "complete"}, err
	}, countingTool(&ran), nil)
	e.registry["code_wait"] = codeWaitTool()
	hub := newCodeHub(t.Context())
	t.Cleanup(hub.close)
	ctx := context.WithValue(t.Context(), codeHubKey{}, hub)
	call := codeTestCall()
	call.Function.Arguments = `{"source":"test","yield_ms":1}`
	ret, _, _ := e.run(ctx, []llm.ToolCall{call}, func(session.ToolData) bool { return true })
	var yielded struct{ ID string }
	require.NoError(t, json.Unmarshal([]byte(ret[0].Content), &yielded))
	require.NotEmpty(t, yielded.ID)
	require.Zero(t, ran.Load())
	wait := llm.ToolCall{
		ID:       "observe",
		Function: llm.Function{Name: "code_wait", Arguments: `{"id":"` + yielded.ID + `"}`},
	}
	other := context.WithValue(t.Context(), codeHubKey{}, newCodeHub(t.Context()))
	ret, _, _ = e.run(other, []llm.ToolCall{wait}, func(session.ToolData) bool { return true })
	require.Contains(t, ret[0].Content, "unavailable in this turn")
	close(ready)
	var events []session.ToolData
	ret, _, _ = e.run(
		ctx,
		[]llm.ToolCall{wait},
		func(event session.ToolData) bool { events = append(events, event); return true },
	)
	require.Equal(t, "complete", ret[0].Content)
	require.EqualValues(t, 1, ran.Load())
	require.True(t, slices.ContainsFunc(events, func(event session.ToolData) bool {
		return event.Run.ParentUseID == "observe" && event.Run.Name == "count"
	}))
	ret, _, _ = e.run(ctx, []llm.ToolCall{wait}, func(session.ToolData) bool { return true })
	require.Contains(t, ret[0].Content, "unavailable in this turn")
}

func TestTerminatingYieldedCodeCancelsWorker(t *testing.T) {
	ended := make(chan struct{})
	var ran atomic.Int32
	e := codeExecutor(func(ctx context.Context, _ codeTestHost) (CodeResult, error) {
		defer close(ended)
		<-ctx.Done()
		return CodeResult{}, ctx.Err()
	}, countingTool(&ran), nil)
	e.registry["code_wait"] = codeWaitTool()
	h := newCodeHub(t.Context())
	t.Cleanup(h.close)
	ctx := context.WithValue(t.Context(), codeHubKey{}, h)
	call := codeTestCall()
	call.Function.Arguments = `{"source":"test","yield_ms":1}`
	msg, _, _ := e.run(ctx, []llm.ToolCall{call}, func(session.ToolData) bool { return true })
	var y struct{ ID string }
	require.NoError(t, json.Unmarshal([]byte(msg[0].Content), &y))
	call.Function = llm.Function{Name: "code_wait", Arguments: `{"id":"` + y.ID + `","terminate":true}`}
	got, _, _ := e.run(ctx, []llm.ToolCall{call}, func(session.ToolData) bool { return true })
	require.Contains(t, got[0].Content, "terminated")
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("terminated worker did not exit")
	}
}
