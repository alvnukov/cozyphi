package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanAutoApproveSurvivesClearAndResume(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		request := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if request%2 == 1 {
			args := fmt.Sprintf(
				`{"steps":[{"content":"continue-%d","status":"in_progress","type":"edit"}]}`,
				request,
			)
			chunk, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"index": 0, "id": fmt.Sprintf("call_%d", request), "type": "function",
						"function": map[string]any{"name": "plan", "arguments": args},
					}},
				}}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		} else {
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	ctrl := newInjectController(t, NewBus(nil), server.URL)
	t.Cleanup(ctrl.Close)
	ctrl.SetPlanAutoApprove(func() bool { return true })
	runControllerPrompt(t, ctrl, &requests, "seed a resumable session", "seed")
	originalSession := ctrl.SessionID()

	require.NoError(t, ctrl.Clear())
	runControllerPrompt(t, ctrl, &requests, "make a plan after clear", "after-clear")
	assert.True(t, ctrl.Plan().Approved, "the replacement engine must retain auto-approval")

	_, err := ctrl.Resume(originalSession)
	require.NoError(t, err)
	runControllerPrompt(t, ctrl, &requests, "make a plan after resume", "after-resume")
	assert.True(t, ctrl.Plan().Approved, "the resumed engine must retain auto-approval")
}

func runControllerPrompt(t *testing.T, ctrl *Controller, requests *atomic.Int32, prompt, id string) {
	t.Helper()
	before := requests.Load()
	ctrl.StartPrompt(prompt, nil, id)
	waitForCond(t, 10*time.Second, func() bool {
		return requests.Load() >= before+2 && !ctrl.RunActive()
	})
}
