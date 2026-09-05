package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
)

func TestExplicitWaitConsumesOutcomeWithoutAnotherWake(t *testing.T) {
	var jobID atomic.Value
	var mu sync.Mutex
	var recorded []string
	bodies := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), recorded...) }
	release := make(chan struct{})
	lateHint := make(chan func(), 1)
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		n := len(recorded)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			args := fmt.Sprintf(`{"job_id":%q}`, jobID.Load().(string))
			_, _ = fmt.Fprintf(
				w,
				"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"wait-child\",\"type\":\"function\",\"function\":{\"name\":\"agent_wait\",\"arguments\":%q}}]}}]}\n\n",
				args,
			)
			defer once.Do(func() { close(release) })
		} else {
			if n == 2 {
				(<-lateHint)() // Receipt is consumed, but final inference has not finished.
			}
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"noted\"}}]}\n\n")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	defer ctrl.Close()
	ctrl.SetMode(agent.ModeBuild)
	ctrl.runtime.EnableInteractiveChildren()
	parentID := ctrl.engine.SessionID()
	info, err := ctrl.jobs.SpawnWithRunner(
		t.Context(),
		job.SpawnRequest{Prompt: "answer", OwnerID: ctrl.jobOwnerID, ParentID: parentID},
		job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			select {
			case <-release:
				return "unique wait result", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}),
	)
	require.NoError(t, err)
	jobID.Store(info.ID)
	lateHint <- func() { ctrl.runtime.notifyOutcome(ctrl.jobOwnerID, parentID) }
	ctrl.StartPrompt("wait for that assignment", nil, "user-wait")
	waitForCond(t, 5*time.Second, func() bool { return len(bodies()) >= 2 && !ctrl.RunActive() })
	require.Equal(
		t,
		1,
		strings.Count(bodies()[1], "unique wait result"),
		"explicit wait and push must not duplicate the result",
	)
	require.Contains(t, bodies()[1], `outcome_id`, "wait response must expose the outcome identity")
	pending, err := ctrl.jobs.PendingOutcomes(t.Context(), ctrl.jobOwnerID, parentID, 1)
	require.NoError(t, err)
	require.Empty(t, pending)
	// A delayed/coalesced hint must reconcile the already-consumed receipt before
	// starting a turn; it is not itself evidence of an unread result.
	ctrl.runtime.notifyOutcome(ctrl.jobOwnerID, parentID)
	time.Sleep(2 * watchWakeDelay)
	require.Len(t, bodies(), 2, "neither final-boundary nor idle stale hints may start another inference")
}
