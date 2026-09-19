package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// TestLoopInjectsQueuedPromptAtToolBoundary pins the mid-turn injection seam:
// a prompt pulled from Inject at a tool-round boundary must reach the model
// inside the SAME turn — the next request already carries it as a user
// message, and UserPromoted tells the UI the queued hint may clear.
func TestLoopInjectsQueuedPromptAtToolBoundary(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		n := len(bodies)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "count", `{}`))
		} else {
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var runs atomic.Int32
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{countingTool(&runs)},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)

	queue := []InjectedPrompt{{Text: "queued question", UserID: "u2", RowOwed: true}}
	var promoted []session.UserPromoted
	for ev := range engine.Loop(t.Context(), "first", LoopOpts{
		Inject: func() []InjectedPrompt {
			out := queue
			queue = nil
			return out
		},
	}) {
		if p, ok := ev.(session.UserPromoted); ok {
			promoted = append(promoted, p)
		}
	}

	mu.Lock()
	got := append([]string(nil), bodies...)
	mu.Unlock()
	require.Len(t, got, 2, "one turn: tool round + final round, no extra run")
	assert.Contains(t, got[1], "queued question",
		"the queued prompt must reach the model at the tool-round boundary, not after the turn ends")
	assert.Equal(t, []session.UserPromoted{{ID: "u2", Text: "queued question"}}, promoted,
		"injection must emit UserPromoted with the display text the moment the model sees the message")
}

// TestLoopPromotesDequeuedOpeningPrompt pins the other delivery point: a
// turn whose opening prompt dequeued from the controller's queue carries
// UserRowOwed with its UserID/UserDisplayText, and the loop yields
// UserPromoted right after the prompt lands in the session. An immediate
// submit carries the same UserID, because that is the id of its session
// entry, but its row is already drawn, so nothing is promoted.
func TestLoopPromotesDequeuedOpeningPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	newEngine := func(t *testing.T) *Engine {
		t.Helper()
		engine, err := NewEngine(EngineOpts{
			Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
			SessionOpts: SessionOpts{Cwd: t.TempDir()},
			Gate:        permission.AllowAll{},
		})
		require.NoError(t, err)
		return engine
	}

	var promoted []session.UserPromoted
	for ev := range newEngine(t).Loop(t.Context(), "follow up", LoopOpts{
		UserID:          "u9",
		UserRowOwed:     true,
		UserDisplayText: "follow up",
	}) {
		if p, ok := ev.(session.UserPromoted); ok {
			promoted = append(promoted, p)
		}
	}
	assert.Equal(t, []session.UserPromoted{{ID: "u9", Text: "follow up"}}, promoted,
		"a dequeued opening prompt must land its transcript row at delivery")

	promoted = nil
	for ev := range newEngine(t).Loop(t.Context(), "direct", LoopOpts{UserID: "u10"}) {
		if p, ok := ev.(session.UserPromoted); ok {
			promoted = append(promoted, p)
		}
	}
	assert.Empty(t, promoted, "an immediate submit already has its row; no promote")
}
