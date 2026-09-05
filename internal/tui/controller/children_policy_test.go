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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
)

func TestInteractiveChildPreservesConfiguredPathDeny(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprintf("deny=%t", deny), func(t *testing.T) {
			var requests atomic.Int32
			var leaked atomic.Bool
			var path string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) == 1 {
					args, _ := json.Marshal(map[string]string{"path": path})
					_, _ = fmt.Fprintf(
						w,
						"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"read-check\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":%q}}]}}]}\n\n",
						args,
					)
				} else {
					leaked.Store(strings.Contains(string(body), "policy-protected fixture content"))
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
			path = filepath.Join(parent.cwd, "protected.txt")
			require.NoError(t, os.WriteFile(path, []byte("policy-protected fixture content"), 0o600))
			if deny {
				parent.proj.Config().Permissions.SensitivePathDeny = []string{path}
			}
			parent.runtime.EnableInteractiveChildren()
			parent.Cancel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
				Prompt: "read the requested file", Role: job.RoleExplore,
				OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
				WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
			}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
			require.NoError(t, err)
			waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
			child := parent.runtime.Children()[0]
			child.Controller.SetAllowAll(true)
			child.Controller.SetMode(agent.ModeBuild)
			child.Ready(nil)
			_, err = parent.jobs.Wait(ctx, info.ID)
			require.NoError(t, err)
			require.EqualValues(t, 2, requests.Load())
			require.Equal(t, !deny, leaked.Load(), "child mode/bypass changes must retain configured path restrictions")
		})
	}
}
