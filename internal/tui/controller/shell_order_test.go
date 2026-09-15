package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImmediateShellCompletionCannotOvertakeNativeToolResult(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		bodies = append(bodies, string(data))
		number := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if number == 1 {
			delta := map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": "immediate-shell", "type": "function", "function": map[string]string{
					"name": "bash", "arguments": `{"command":"echo immediate_result","run_in_background":true}`,
				},
			}}}
			payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		} else {
			_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant","content":"observed"}}]}`+"\n\n")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	c := newInjectController(t, NewBus(nil), server.URL)
	defer c.Close()
	c.SetAllowAll(true)
	c.SetPlanEnabled(false)
	c.StartPrompt("run the approved background shell", nil)
	snapshot := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), bodies...) }
	waitForCond(t, 8*time.Second, func() bool {
		requests := snapshot()
		for _, request := range requests {
			if strings.Contains(request, "No human input has occurred") {
				return !c.RunActive()
			}
		}
		return false
	})
	var notification string
	for _, body := range snapshot() {
		if strings.Contains(body, "No human input has occurred") {
			notification = body
			break
		}
	}
	require.NotEmpty(t, notification)
	native := strings.Index(notification, `"role":"tool"`)
	receipt := strings.Index(notification, "No human input has occurred")
	require.NotEqual(t, -1, native, "the native background acceptance must already be in context")
	require.Less(t, native, receipt, "a terminal notification cannot precede the executor's tool result")
	pending, err := c.shellTasks.Pending(c.SessionID(), 1)
	require.NoError(t, err)
	require.Empty(t, pending)
}
