package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// midTurnSSEServer: request 1 streams a tool call and then blocks until
// released, so the test can queue a prompt while the turn is genuinely
// mid-round. Request 2 answers with plain text.
func midTurnSSEServer(t *testing.T, readFile string) (srv *httptest.Server, bodies func() []string, release func()) {
	t.Helper()
	releaseCh := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(releaseCh) }) }

	var mu sync.Mutex
	var recorded []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		n := len(recorded)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			args, _ := json.Marshal(map[string]string{"path": readFile})
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []any{map[string]any{
							"index": 0, "id": "call_1", "type": "function",
							"function": map[string]any{"name": "read", "arguments": string(args)},
						}},
					},
				}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case <-releaseCh:
			case <-r.Context().Done():
				return
			}
		} else {
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"answered queued\"}}]}\n\n")
		}
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	bodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
	return srv, bodies, release
}

// TestController_QueueInjectsMidTurn is the user's exact symptom at the
// controller seam: a prompt submitted while the model is tool-looping must
// reach the model on the NEXT tool-round boundary — inside the same turn —
// and UserPromoted must clear the queued hint at that moment, not when the
// whole turn eventually ends.
func TestController_QueueInjectsMidTurn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")

	cwd := t.TempDir()
	readFile := filepath.Join(cwd, "note.txt")
	require.NoError(t, os.WriteFile(readFile, []byte("hello"), 0o644))

	srv, bodies, release := midTurnSSEServer(t, readFile)
	defer srv.Close()
	defer release()
	t.Setenv("COZYPHI_BASE_URL", srv.URL)

	bus := NewBus(nil)
	ctrl := newReadyController(t)
	ctrl.SetAllowAll(true)
	t.Cleanup(ctrl.Close)

	ctrl.StartPrompt("first", nil, "u1")
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) >= 1 })

	// Queue a prompt while round 1 is blocked mid-stream.
	ctrl.StartPrompt("queued question", nil, "u2")

	// The queue must not leak into a finished state: marker hint clears and
	// the model sees the message in the very next request.
	release()

	waitForCond(t, 10*time.Second, func() bool { return len(bodies()) >= 2 })
	got := bodies()
	require.Len(t, got, 2, "mid-turn injection must reuse the running turn, not start a new one")
	assert.Contains(t, got[1], "queued question",
		"the second model request must already carry the queued prompt")

	promoted := ""
	deadline := time.Now().Add(5 * time.Second)
	for promoted == "" && time.Now().Before(deadline) {
		for _, msg := range bus.Drain() {
			event, ok := msg.(SessionEventMsg)
			if !ok {
				continue
			}
			if p, ok := event.Event.(session.UserPromoted); ok {
				promoted = p.ID
			}
		}
		if promoted == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	assert.Equal(t, "u2", promoted, "UserPromoted must fire when the model sees the message")
}

func waitForCond(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
