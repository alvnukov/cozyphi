package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

// capturingTextServer records every chat-completions request body and answers
// with a plain text round.
func capturingTextServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			b = nil
		}
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk(), "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func drainLoop(t *testing.T, engine *Engine, prompt string) {
	t.Helper()
	for ev, err := range engine.Loop(t.Context(), prompt, LoopOpts{}) {
		if err != nil {
			t.Fatalf("loop: %v", err)
		}
		if up, ok := ev.(session.AssistantMessageUpdate); ok && up.Message.State == session.StateComplete {
			return
		}
	}
}

// TestSetModePlanSwapsPromptAndTools covers the plan posture end to end: the
// write/edit tools leave the tool list, the system prompt gains the plan
// appendix, and switching back to build restores both.
func TestSetModePlanSwapsPromptAndTools(t *testing.T) {
	server, bodies := capturingTextServer(t)

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	require.Equal(t, ModeUsePlan, engine.Mode())
	require.True(t, engine.HasTool("write"))
	require.True(t, engine.HasTool("edit"))

	engine.SetMode(ModePlan)
	require.Equal(t, ModePlan, engine.Mode())
	require.False(t, engine.HasTool("write"), "plan mode must not offer write")
	require.False(t, engine.HasTool("edit"), "plan mode must not offer edit")
	require.True(t, engine.HasTool("read"))
	require.True(t, engine.HasTool("bash"), "plan keeps bash; the readonly gate folds it to the allowlist")

	drainLoop(t, engine, "plan this")
	require.NotEmpty(t, bodies())
	lastReq := bodies()[len(bodies())-1]
	require.Contains(t, lastReq, "plan mode")
	require.Contains(t, lastReq, "numbered plan")

	engine.SetMode(ModeBuild)
	require.True(t, engine.HasTool("write"))
	drainLoop(t, engine, "build this")
	lastReq = bodies()[len(bodies())-1]
	require.NotContains(t, lastReq, "plan mode", "build prompt must not carry the plan appendix")
}

// TestSetModePlanKeepsCustomTools pins that plan mode only narrows the
// built-in tool set: an engine assembled with explicit tools keeps them.
func TestSetModePlanKeepsCustomTools(t *testing.T) {
	server, _ := capturingTextServer(t)

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{countingTool(&atomic.Int32{})},
	})
	require.NoError(t, err)

	engine.SetMode(ModePlan)
	require.True(t, engine.HasTool("count"))
}
