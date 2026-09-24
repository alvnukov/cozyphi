package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func lifecycleEngine(t *testing.T, url, parentID string) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:          llm.ModelConfig{Name: "fake", BaseURL: url, APIKey: "x", ContextWindow: 100000},
		SessionOpts:    SessionOpts{Cwd: t.TempDir(), ParentID: parentID},
		LifecycleHooks: true,
	})
	require.NoError(t, err)
	return engine
}

func TestQueuedSessionContextIsDeliveredOnce(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := lifecycleEngine(t, server.URL, "")

	engine.QueueSessionContext("FIRST-BOOT-SENTINEL")
	engine.QueueSessionContext("SECOND-BOOT-SENTINEL")
	drainLoop(t, engine, "hello")
	drainLoop(t, engine, "again")

	got := bodies()
	require.Len(t, got, 2)
	require.NotContains(t, got[0], "FIRST-BOOT-SENTINEL", "a later queue replaces the earlier one")
	require.Equal(t, 1, strings.Count(got[0], "SECOND-BOOT-SENTINEL"))
	// The body is JSON, which escapes '<': decode it to see the prompt as sent.
	var request struct {
		Messages []llm.Message `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(got[0]), &request))
	require.NotEmpty(t, request.Messages)
	prompt := request.Messages[len(request.Messages)-1]
	require.Equal(t, llm.RoleUser, prompt.Role)
	require.Contains(t, prompt.Content, "<system-reminder>\nSECOND-BOOT-SENTINEL\n</system-reminder>")
	require.Equal(t, 1, strings.Count(got[1], "SECOND-BOOT-SENTINEL"), "history carries it; nothing re-injects it")
}

func TestSessionContextRidesTheFirstToolResult(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused", func(n int32) string {
		if n == 1 {
			return sseToolCallChunk("call_1", "context", `{}`)
		}
		return sseTextChunk()
	})
	engine := lifecycleEngine(t, server.URL, "")

	var queued bool
	for _, err := range engine.Loop(t.Context(), "hello", LoopOpts{}) {
		require.NoError(t, err)
		// The first request is on the wire, so the prompt is already composed;
		// Loop is a pull iterator, so the tool has not run yet.
		if !queued && streams.Load() >= 1 {
			engine.QueueSessionContext("MID-TURN-SENTINEL")
			queued = true
		}
	}
	require.True(t, queued)
	all := bodies()
	require.GreaterOrEqual(t, len(all), 2)
	require.NotContains(t, all[0], "MID-TURN-SENTINEL")
	require.Equal(t, 1, strings.Count(all[1], "MID-TURN-SENTINEL"), "the tool result boundary delivers it")
}

func TestChildEngineIgnoresSessionContext(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine := lifecycleEngine(t, server.URL, "parent-session")

	engine.QueueSessionContext("CHILD-SENTINEL")
	drainLoop(t, engine, "hello")
	require.NotContains(t, bodies()[0], "CHILD-SENTINEL")
}

func TestEngineWithoutLifecycleIgnoresSessionContext(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	engine.QueueSessionContext("OFF-SENTINEL")
	drainLoop(t, engine, "hello")
	require.NotContains(t, bodies()[0], "OFF-SENTINEL")
}
