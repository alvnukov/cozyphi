package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestInteractiveWorkerAskIsOriginBoundAndCancelledReplyCannotExecute(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprintf("interrupt=%t", interrupt), func(t *testing.T) {
			var requests atomic.Int32
			var path string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) == 1 {
					args, _ := json.Marshal(map[string]string{"command": fmt.Sprintf("printf approved > %q", path)})
					_, _ = fmt.Fprintf(
						w,
						"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"write-check\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":%q}}]}}]}\n\n",
						args,
					)
				} else {
					_, _ = fmt.Fprint(
						w,
						"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"finished\"}}]}\n\n",
					)
				}
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			parent := newInjectController(t, NewBus(nil), server.URL)
			defer parent.Close()
			path = filepath.Join(parent.cwd, "approved.txt")
			policy := &parent.proj.Config().Permissions
			policy.BashAllow, policy.BashDeny = nil, nil
			policy.BashDefault = permission.Ask
			parent.SetAllowAll(true) // The parent's approval is not the child's approval.
			parent.runtime.EnableInteractiveChildren()
			parent.Cancel()
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
				Prompt: "perform the operation", Role: job.RoleWorker,
				OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
				WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
			}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
			require.NoError(t, err)
			waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
			child := parent.runtime.Children()[0]
			child.Controller.SetMode(agent.ModeBuild)
			child.Ready(nil)
			var ask PermissionAskMsg
			waitForCond(t, 5*time.Second, func() bool {
				for _, msg := range child.Bus.Drain() {
					if request, ok := msg.(PermissionAskMsg); ok {
						ask = request
					}
				}
				return ask.Reply != nil
			})
			for _, msg := range parent.bus.Drain() {
				_, wrong := msg.(PermissionAskMsg)
				require.False(t, wrong)
			}
			_, err = os.Stat(path)
			require.True(t, os.IsNotExist(err), "no execution before the child's human approval")
			if interrupt {
				child.Controller.Cancel()
				waitForCond(
					t,
					5*time.Second,
					func() bool { return child.Controller.Assignment().Turn == TurnInterrupted },
				)
				ask.Reply <- AskReply{Approved: true} // A stale overlay cannot revive execution.
				child.Controller.LeaveAssignment()
			} else {
				ask.Reply <- AskReply{Approved: true}
			}
			result, err := parent.jobs.Wait(ctx, info.ID)
			require.NoError(t, err)
			require.True(t, result.Info.UserIntervened)
			outcomes, err := parent.jobs.PendingOutcomes(ctx, parent.jobOwnerID, parent.engine.SessionID(), 4)
			require.NoError(t, err)
			require.Len(t, outcomes, 1)
			require.True(t, outcomes[0].UserIntervened)
			data, err := os.ReadFile(path)
			if interrupt {
				require.Equal(t, job.StatusCancelled, result.Info.Status)
				require.True(t, os.IsNotExist(err))
			} else {
				require.NoError(t, err)
				require.Equal(t, "approved", string(data))
				require.Equal(t, job.StatusCompleted, result.Info.Status)
			}
		})
	}
}
