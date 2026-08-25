package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// TestLoopMapsFinishLengthToMaxTokens: a provider that reports finish_reason
// "length" exhausted the output budget; the round must end StopMaxTokens so
// the transcript can say the answer was cut off instead of silently missing.
func TestLoopMapsFinishLengthToMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","content":"reasoning and more reasoning"},"finish_reason":"length"}]}`+"\n\n",
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
	require.Equal(t, session.StopMaxTokens, complete.StopReason)
}
