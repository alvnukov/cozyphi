package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// TestLoopStampsThinkingDuration: the round's reasoning span (first reasoning
// delta → first text delta) must reach the session message so the thinking
// header can say "Thought for Xs" instead of a perpetual "Thinking".
func TestLoopStampsThinkingDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"reasoning_content":"weighing options"}}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"the answer"}}]}`+"\n\n")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","content":"the answer"},"finish_reason":"stop"}]}`+"\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)

	var complete *session.Message
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		require.NoError(t, err)
		if update, ok := ev.(session.AssistantMessageUpdate); ok && update.Message.State == session.StateComplete {
			msg := update.Message
			complete = &msg
		}
	}
	require.NotNil(t, complete, "expected a complete round")
	require.Positive(t, complete.ThinkingDuration.Nanoseconds())
	require.Less(t, complete.ThinkingDuration, 10*time.Second, "span must be wall-clock plausible")
}

// TestLoopThinkingDurationZeroWithoutReasoning: rounds without reasoning
// report no thinking span (zero renders as a bare "Thought").
func TestLoopThinkingDurationZeroWithoutReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","content":"plain"},"finish_reason":"stop"}]}`+"\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var runs atomic.Int32
	engine := newRoundTestEngine(t, server.URL, &runs)

	var complete *session.Message
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		require.NoError(t, err)
		if update, ok := ev.(session.AssistantMessageUpdate); ok && update.Message.State == session.StateComplete {
			msg := update.Message
			complete = &msg
		}
	}
	require.NotNil(t, complete, "expected a complete round")
	require.Zero(t, complete.ThinkingDuration)
}
