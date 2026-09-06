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

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestInteractiveChildPublishesProgressToParent: a child the user can open and
// talk to still reports its tool rows to the parent's transcript, exactly as a
// headless one does — the same job.Progress, addressed to the spawn row that
// made it, arriving on the parent's bus as JobProgressMsg. Observed through
// the manager and the bus, never through the child's internals.
func TestInteractiveChildPublishesProgressToParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	cwd := t.TempDir()
	notes := filepath.Join(cwd, "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("hello\n"), 0o600))

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			args, _ := json.Marshal(map[string]string{"path": notes})
			payload := fmt.Sprintf(
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"child-read","type":"function","function":{"name":"read","arguments":%q}}]}}]}`,
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
	bus := NewBus(nil)
	parent, err := runtime.NewSession(bus, workspace, "", nil)
	require.NoError(t, err)
	runner := parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	info, err := runtime.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "read the notes", Role: job.RoleExplore,
		ParentID: parent.SessionID(), OwnerID: parent.jobOwnerID, ParentToolUseID: "call_agent",
		WorkDir: cwd, ParentWorkspace: cwd,
	}, runner)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(runtime.Children()) == 1 }, 5*time.Second, time.Millisecond)
	child := runtime.Children()[0]
	child.Controller.SetAllowAll(true)
	child.Ready(nil)

	result, err := runtime.jobs.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCompleted, result.Info.Status)
	require.Equal(t, "child answer", result.Summary)

	// The row the parent shows is the child's read, and it settles: the loop
	// ends on the terminal status, so reaching it proves everything before it
	// was delivered too.
	seen := make(map[string]int)
	var settled bool
	deadline := time.After(10 * time.Second)
	for !settled {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				p, ok := msg.(JobProgressMsg)
				if !ok {
					continue
				}
				require.Equal(t, info.ID, p.Progress.JobID)
				require.Equal(t, "call_agent", p.Progress.ParentToolUseID)
				require.Equal(t, parent.SessionID(), p.Progress.ParentID)
				require.Equal(t, "child-read", p.Progress.ToolUseID)
				require.Equal(t, "read", p.Progress.Name)
				require.NotEmpty(t, p.Progress.Detail, "a child row names what it worked on")
				seen[p.Progress.Status+"\x00"+p.Progress.Detail]++
				settled = settled || p.Progress.Status == session.ToolDone.String()
			}
		case <-deadline:
			t.Fatalf("the parent never heard the child's tool row: %v", seen)
		}
	}
	for sig, n := range seen {
		require.Equalf(t, 1, n, "dedupe must publish each state once: %q", sig)
	}

	// The hook lives with the assignment: the child is retained, but its
	// assignment is over, so it reports nothing more.
	require.True(t, child.Controller.Assignment().Terminal)
	child.Controller.Close()
}
