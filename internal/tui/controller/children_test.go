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

	"github.com/stretchr/testify/assert"
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
	firstRequest := make(chan struct{})
	releaseRequest := make(chan struct{})
	requestBodies := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requestBodies <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			close(firstRequest)
			select {
			case <-releaseRequest:
			case <-r.Context().Done():
				return
			}
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
	// A sibling remains attached but idle while this child selects its own model.
	siblingInfo, err := runtime.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "wait", Role: job.RoleExplore, ParentID: "parent-conversation",
		WorkDir: cwd, ParentWorkspace: cwd,
	}, runner)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(runtime.Children()) == 2 }, 5*time.Second, time.Millisecond)
	var sibling *Controller
	for _, entry := range runtime.Children() {
		if entry.JobID == siblingInfo.ID {
			sibling = entry.Controller
		}
	}
	require.NotNil(t, sibling)
	parentStatus, siblingStatus := parent.ModelSelectionStatus(), sibling.ModelSelectionStatus()
	child.Ready(nil)
	select {
	case <-firstRequest:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, child.Controller.SetModelEffort("child-next", ""))
	status := child.Controller.ModelSelectionStatus()
	require.Equal(t, "child-next", status.Selected.Name)
	require.Equal(t, "test-model", status.Effective.Name)
	require.True(t, status.Pending)
	require.Error(t, child.Controller.SetModelEffort("unsupported", "high"))
	require.Equal(t, status, child.Controller.ModelSelectionStatus())
	require.Equal(t, parentStatus, parent.ModelSelectionStatus())
	require.Equal(t, siblingStatus, sibling.ModelSelectionStatus())
	sibling.Close()
	close(releaseRequest)
	result, err := runtime.jobs.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCompleted, result.Info.Status)
	require.Equal(t, "child answer", result.Summary)
	require.Contains(t, <-requestBodies, `"model":"test-model"`)
	require.Contains(t, <-requestBodies, `"model":"child-next"`)
	require.False(t, child.Controller.ModelSelectionStatus().Pending)
	_, err = os.Stat(forbidden)
	require.True(t, os.IsNotExist(err), "interactive role ceiling must survive mode and bypass changes")
	require.Same(t, child.Controller, runtime.Children()[0].Controller)
	require.True(t, child.Controller.Assignment().Terminal)
	child.Controller.Close()
	require.Empty(t, runtime.Children(), "explicitly closed children must not retain their engine/View graph")
	_, err = runtime.jobs.Wait(ctx, info.ID)
	require.NoError(t, err, "closing a retained View preserves durable job history")
}

// The job manager's list spans every session on disk, so /agents keeps only
// the jobs this conversation spawned — and a session with no id claims none of
// them rather than claiming the parentless ones.
func TestChildJobsKeepsOnlyThisSessionsChildren(t *testing.T) {
	all := []job.Info{
		{Meta: job.Meta{ID: "mine-1", ParentID: "sess-a"}},
		{Meta: job.Meta{ID: "theirs", ParentID: "sess-b"}},
		{Meta: job.Meta{ID: "orphan"}},
		{Meta: job.Meta{ID: "mine-2", ParentID: "sess-a"}},
	}
	got := childrenOf(all, "sess-a")
	require.Len(t, got, 2)
	assert.Equal(t, "mine-1", got[0].ID)
	assert.Equal(t, "mine-2", got[1].ID)

	assert.Empty(t, childrenOf(all, ""), "a session with no id adopts nothing")
}
