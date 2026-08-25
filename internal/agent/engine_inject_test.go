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

	queue := []InjectedPrompt{{Text: "queued question", UserID: "u2"}}
	var promoted []string
	for ev := range engine.Loop(t.Context(), "first", LoopOpts{
		Inject: func() []InjectedPrompt {
			out := queue
			queue = nil
			return out
		},
	}) {
		if p, ok := ev.(session.UserPromoted); ok {
			promoted = append(promoted, p.ID)
		}
	}

	mu.Lock()
	got := append([]string(nil), bodies...)
	mu.Unlock()
	require.Len(t, got, 2, "one turn: tool round + final round, no extra run")
	assert.Contains(t, got[1], "queued question",
		"the queued prompt must reach the model at the tool-round boundary, not after the turn ends")
	assert.Equal(t, []string{"u2"}, promoted,
		"injection must emit UserPromoted so the UI clears the queued hint the moment the model sees the message")
}
