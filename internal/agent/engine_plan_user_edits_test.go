package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestUserPlanSaveRejectsStaleProviderRound(t *testing.T) {
	for _, tc := range []struct{ name, tool, args string }{
		{name: "patch", tool: "plan", args: `{"action":"patch","ops":[{"op":"set_plan_fields","goal":"obsolete model goal"}]}`},
		{name: "create", tool: "plan", args: `{"action":"create","goal":"obsolete","approach":"old","successCriteria":["old"],"steps":[{"id":"x","content":"old","type":"explore","status":"pending","why":"old","doneWhen":"old"}]}`},
		{name: "update", tool: "plan", args: `{"action":"update","items":[{"content":"old","status":"pending","type":"explore"}]}`},
		{name: "working", tool: "count", args: `{}`},
		{name: "envelope", tool: "count", args: `{"_plan":{"workingContext":"obsolete context"}}`},
		{name: "text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			textOnly := tc.tool == ""
			var runs atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			var requests atomic.Int32
			var nextRequest string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&body)
				n := requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if n == 1 {
					close(entered)
					<-release
					if textOnly {
						_, _ = fmt.Fprint(w, sseTextChunk())
					} else {
						_, _ = fmt.Fprint(w, sseToolCallChunk("stale", tc.tool, tc.args))
					}
				} else {
					nextRequest = string(body)
					_, _ = fmt.Fprint(w, sseTextChunk())
				}
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			engine, err := NewEngine(
				EngineOpts{
					Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
					SessionOpts: SessionOpts{Cwd: t.TempDir()},
					Tools:       []tools.Tool{countingTool(&runs)},
				},
			)
			require.NoError(t, err)
			plan, _, _, err := engine.createPlan(t.Context(), seedContract())
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
					if err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			<-entered
			_, _, err = engine.PatchPlanFromUser(
				t.Context(),
				plan.Revision,
				[]session.PlanPatchOp{
					{
						Op:   session.PlanPatchSetPlanFields,
						Goal: session.PatchValue[string]{Set: true, Value: "user priority goal"},
					},
				},
			)
			close(release)
			require.NoError(t, err)
			require.NoError(t, <-done)
			require.Equal(t, "user priority goal", engine.Plan().Goal)
			require.Zero(t, runs.Load())
			require.EqualValues(t, 2, requests.Load())
			require.Contains(t, nextRequest, "user priority goal")
			require.Contains(t, nextRequest, "take priority")
			if !textOnly {
				require.Contains(t, nextRequest, "stale model round")
			}
		})
	}
}
