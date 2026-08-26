package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// sseToolCallChunk encodes one SSE data line carrying a full tool-call delta.
func sseToolCallChunk(id, name, args string) string {
	payload, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []any{map[string]any{
					"index":    0,
					"id":       id,
					"type":     "function",
					"function": map[string]any{"name": name, "arguments": args},
				}},
			},
		}},
	})
	if err != nil {
		panic(err)
	}
	return "data: " + string(payload) + "\n\n"
}

func sseTextChunk() string {
	payload, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"role":    "assistant",
				"content": "done",
			},
		}},
	})
	if err != nil {
		panic(err)
	}
	return "data: " + string(payload) + "\n\n"
}

// fakeToolSequenceServer returns tool calls for finalAfter tool requests, then
// returns a final text response. A negative finalAfter means tool calls forever.
func fakeToolSequenceServer(finalAfter int) (*httptest.Server, *atomic.Int32) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		request := requests.Add(1)
		if finalAfter < 0 || int(request) <= finalAfter {
			_, _ = fmt.Fprint(w, sseToolCallChunk(fmt.Sprintf("call_%d", request), "count", `{}`))
		} else {
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	return server, &requests
}

func countingTool(runs *atomic.Int32) tools.Tool {
	return tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "count",
			Description: "count tool executions",
			Params:      &llm.FunctionParameters{Type: "object"},
		},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			runs.Add(1)
			return tools.Result{Content: "ok"}, nil
		},
	}
}

func newRoundTestEngine(t *testing.T, serverURL string, runs *atomic.Int32) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: serverURL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		Tools:       []tools.Tool{countingTool(runs)},
	})
	require.NoError(t, err)
	return engine
}

func TestLoopConsumerStopDuringStreamErrorDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintln(
			w,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
		)
		_, _ = fmt.Fprintln(w)
		// Exceed the SSE scanner limit after a valid delta so streamTurn has
		// partial output when the provider reports an error.
		_, _ = fmt.Fprintf(w, "data: %s\n", strings.Repeat("x", 10*1024*1024))
	}))
	defer server.Close()

	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{
			Name: "claude-test", Protocol: llm.ProtocolAnthropic, BaseURL: server.URL, APIKey: "x",
		},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)

	sawErrorState := false
	require.NotPanics(t, func() {
		for ev := range engine.Loop(t.Context(), "go", LoopOpts{}) {
			update, ok := ev.(session.AssistantMessageUpdate)
			if ok && update.Message.State == session.StateError {
				sawErrorState = true
				break
			}
		}
	})
	require.True(t, sawErrorState)
}

func TestLoopConsumerStopDuringToolEventDoesNotPanic(t *testing.T) {
	server, _ := fakeToolSequenceServer(-1)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)

	sawToolStart := false
	require.NotPanics(t, func() {
		for ev := range engine.Loop(t.Context(), "go", LoopOpts{}) {
			toolData, ok := ev.(session.ToolData)
			if ok && toolData.Run.Status == session.ToolInProgress {
				sawToolStart = true
				break
			}
		}
	})
	require.True(t, sawToolStart)
	require.Zero(t, runs.Load(), "breaking the event stream must stop the pending tool")

	context := engine.session.BuildContext()
	require.Len(t, context, 3, "the persisted tool_use must be closed even after the event consumer stops")
	require.Equal(t, llm.RoleTool, context[2].Role)
	require.Equal(t, "call_1", context[2].ToolCallID)
	require.Equal(t, ToolCanceledResult, context[2].Content)
}

func TestLoopConsumerStopAfterToolCompletionDoesNotContinue(t *testing.T) {
	server, requests := fakeToolSequenceServer(-1)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)

	sawToolDone := false
	require.NotPanics(t, func() {
		for ev := range engine.Loop(t.Context(), "go", LoopOpts{}) {
			toolData, ok := ev.(session.ToolData)
			if ok && toolData.Run.Status == session.ToolDone {
				sawToolDone = true
				break
			}
		}
	})
	require.True(t, sawToolDone)
	require.Equal(t, int32(1), runs.Load())
	require.Equal(t, int32(1), requests.Load(), "consumer stop must prevent the next model round")
}

func TestLoopConsumerStopBeforeCompactionDoesNotPanic(t *testing.T) {
	server, streams, _ := fakeContextServer(t, "unused", func(int32) string {
		return sseToolCallChunk("call_1", "context", `{"action":"compact"}`)
	})
	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	sawCompactionStart := false
	require.NotPanics(t, func() {
		for ev := range engine.Loop(t.Context(), "go", LoopOpts{}) {
			if _, ok := ev.(session.CompactionStarted); ok {
				sawCompactionStart = true
				break
			}
		}
	})
	require.True(t, sawCompactionStart)
	require.Equal(t, int32(1), streams.Load(), "consumer stop must prevent the next model round")
}

func TestCompactNowConsumerStopReturnsCanceled(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	events := 0
	err := engine.CompactNow(t.Context(), func(session.Event) bool {
		events++
		return false
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, events)
	require.Zero(t, streams.Load())
	require.Empty(t, bodies(), "consumer stop must prevent the compaction request")
}

func TestLoopMaxRoundsAllowsFinalAnswerAfterLastToolRound(t *testing.T) {
	server, requests := fakeToolSequenceServer(2)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)
	require.NoError(t, engine.SetMaxRounds(2))

	var lastErr error
	var finalText string
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		if update, ok := ev.(session.AssistantMessageUpdate); ok && update.Message.State == session.StateComplete {
			finalText = update.Message.FlatText()
		}
	}
	require.NoError(t, lastErr)
	require.Equal(t, int32(2), runs.Load())
	require.Equal(t, int32(3), requests.Load())
	require.Equal(t, "done", finalText)
}

func TestLoopMaxRoundsDoesNotExecuteExtraToolRound(t *testing.T) {
	server, requests := fakeToolSequenceServer(-1)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)
	require.NoError(t, engine.SetMaxRounds(2))

	var lastErr error
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		_ = ev
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr, "loop should stop when the model requests a third tool round")
	if !errors.Is(lastErr, ErrMaxRounds) {
		t.Fatalf("expected ErrMaxRounds to be wrapped, got %v", lastErr)
	}
	require.Equal(t, int32(2), runs.Load())
	require.Equal(t, int32(3), requests.Load())

	assistantToolRounds := 0
	for _, msg := range engine.session.BuildContext() {
		if msg.Role == llm.RoleAssistant && len(msg.ToolCalls) > 0 {
			assistantToolRounds++
		}
	}
	require.Equal(t, 2, assistantToolRounds)
}

func TestLoopStopOnLimitDisabledRunsToCompletion(t *testing.T) {
	server, requests := fakeToolSequenceServer(2)
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)
	require.NoError(t, engine.SetMaxRounds(1))
	engine.SetStopOnLimit(false)

	var lastErr error
	var finalText string
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		if update, ok := ev.(session.AssistantMessageUpdate); ok && update.Message.State == session.StateComplete {
			finalText = update.Message.FlatText()
		}
	}
	require.NoError(t, lastErr, "disabling the budget must not cap tool rounds")
	require.Equal(t, int32(2), runs.Load())
	require.Equal(t, int32(3), requests.Load())
	require.Equal(t, "done", finalText)
}

func TestLoopContinueAskGrantsAnotherBudget(t *testing.T) {
	server, _ := fakeToolSequenceServer(-1)
	defer server.Close()

	var asks atomic.Int32
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		ContinueAsk: func(context.Context, int) (bool, error) {
			// Approve once so the loop can start a second budget window, then stop.
			return asks.Add(1) == 1, nil
		},
	})
	require.NoError(t, err)
	require.NoError(t, engine.SetMaxRounds(1))

	var lastErr error
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		_ = ev
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr)
	require.ErrorIs(t, lastErr, ErrMaxRounds)
	require.Equal(t, int32(2), asks.Load(), "should ask once per exhausted budget")
}

func TestLoopContinueAskDeclineReturnsErrMaxRounds(t *testing.T) {
	server, _ := fakeToolSequenceServer(-1)
	defer server.Close()

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
		ContinueAsk: func(context.Context, int) (bool, error) {
			return false, nil
		},
	})
	require.NoError(t, err)
	require.NoError(t, engine.SetMaxRounds(1))

	var lastErr error
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		_ = ev
		if err != nil {
			lastErr = err
			break
		}
	}
	require.ErrorIs(t, lastErr, ErrMaxRounds)
}

func TestSetMaxRoundsRejectsNonPositive(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://unused", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)
	require.Error(t, engine.SetMaxRounds(0))
	require.Error(t, engine.SetMaxRounds(-1))
	require.NoError(t, engine.SetMaxRounds(1))
}
