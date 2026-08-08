package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/stretchr/testify/require"
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

// fakeToolLoopServer always asks the model to run `echo hi` so a Loop never
// ends on its own; used to prove the max-rounds budget fires.
func fakeToolLoopServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "bash", `{"command":"echo hi"}`))
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func TestLoopExceedsMaxRounds(t *testing.T) {
	server := fakeToolLoopServer()
	defer server.Close()

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)
	require.NoError(t, engine.SetMaxRounds(2))

	var lastErr error
	for ev, err := range engine.Loop(context.Background(), "go", LoopOpts{}) {
		_ = ev
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Error(t, lastErr, "loop should stop with an error once rounds are exhausted")
	if !errors.Is(lastErr, ErrMaxRounds) {
		t.Fatalf("expected ErrMaxRounds to be wrapped, got %v", lastErr)
	}
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
