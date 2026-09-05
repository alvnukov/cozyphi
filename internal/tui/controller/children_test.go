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
	"github.com/alvnukov/cozyphi/internal/project"
)

// The adapter is assembled here; behavior is observed through Manager and
// retained Controller APIs, not through its internal scheduling state.
func TestInteractiveRunnerRetainsChildAndRoleCeiling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	cwd := t.TempDir()
	forbidden := filepath.Join(cwd, "must-not-exist")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			args, _ := json.Marshal(map[string]string{"command": "touch " + forbidden})
			payload := fmt.Sprintf(
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"write-attempt","type":"function","function":{"name":"bash","arguments":%q}}]}}]}`,
				string(args),
			)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		} else {
			_, _ = fmt.Fprint(
				w,
				"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"child answer\"}}]}\n\n",
			)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	t.Setenv("COZYPHI_BASE_URL", server.URL)
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	runtime, err := NewRuntime(proj)
	require.NoError(t, err)
	defer runtime.Close()
	runtime.EnableInteractiveChildren()
	workspace, err := runtime.Workspace(cwd)
	require.NoError(t, err)
	parent, err := runtime.NewSession(NewBus(nil), workspace, "", nil)
	require.NoError(t, err)
	runner := parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	info, err := runtime.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "try the operation", Role: job.RoleExplore, ParentID: "parent-conversation",
		WorkDir: cwd, ParentWorkspace: cwd,
	}, runner)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(runtime.Children()) == 1 }, 5*time.Second, time.Millisecond)
	child := runtime.Children()[0]
	require.Equal(t, info.ID, child.JobID)
	require.Zero(t, requests.Load(), "inference waits for complete View attachment")
	require.False(t, child.Controller.AgentsEnabled())
	child.Controller.SetAllowAll(true)
	child.Controller.SetMode(agent.ModeBuild)
	child.Ready(nil)
	result, err := runtime.jobs.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCompleted, result.Info.Status)
	require.Equal(t, "child answer", result.Summary)
	_, err = os.Stat(forbidden)
	require.True(t, os.IsNotExist(err), "interactive role ceiling must survive mode and bypass changes")
	require.Same(t, child.Controller, runtime.Children()[0].Controller)
	require.True(t, child.Controller.Assignment().Terminal)
	child.Controller.Close()
	require.Empty(t, runtime.Children(), "explicitly closed children must not retain their engine/View graph")
	_, err = runtime.jobs.Wait(ctx, info.ID)
	require.NoError(t, err, "closing a retained View preserves durable job history")
}
