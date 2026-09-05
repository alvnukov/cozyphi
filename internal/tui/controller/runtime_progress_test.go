package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
)

func TestRuntimeRoutesJobProgressOnlyToOriginatingSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(rt.Close)
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	busA, busB := NewBus(nil), NewBus(nil)
	a, err := rt.NewSession(busA, ws, "", nil)
	require.NoError(t, err)
	b, err := rt.NewSession(busB, ws, "", nil)
	require.NoError(t, err)

	// Spawn through the shared manager's public adapter seam. Ordered terminal
	// writes form a barrier: reaching each owner's last progress proves its
	// subscription has also processed and rejected the earlier sibling events.
	emit := func(parent, owner, marker string) {
		t.Helper()
		info, err := rt.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
			Prompt: marker, ParentID: parent, OwnerID: owner,
		}, job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			env.OnProgress(job.Progress{
				ParentID: "forged-origin", OwnerID: "forged-owner", ToolUseID: marker, Name: "read", Status: "done",
			})
			return marker, nil
		}))
		require.NoError(t, err)
		_, err = rt.jobs.Wait(t.Context(), info.ID)
		require.NoError(t, err)
	}
	emit(a.SessionID(), a.jobOwnerID, "a-first")
	emit(b.SessionID(), b.jobOwnerID, "b-first")
	// A different live owner may open the same persisted history. Correlation
	// alone must not grant it access to this controller's progress stream.
	emit(a.SessionID(), b.jobOwnerID, "foreign-owner")
	emit(a.SessionID(), a.jobOwnerID, "a-last")
	emit(b.SessionID(), b.jobOwnerID, "b-last")

	collect := func(bus *Bus, owner, last string) []string {
		t.Helper()
		var markers []string
		deadline := time.After(5 * time.Second)
		for {
			select {
			case <-bus.Chan():
				for _, msg := range bus.Drain() {
					if p, ok := msg.(JobProgressMsg); ok {
						require.Equal(t, owner, p.Progress.ParentID)
						markers = append(markers, p.Progress.ToolUseID)
						if p.Progress.ToolUseID == last {
							return markers
						}
					}
				}
			case <-deadline:
				t.Fatal("timed out waiting for owned job progress")
				return nil
			}
		}
	}
	require.Equal(t, []string{"a-first", "a-last"}, collect(busA, a.SessionID(), "a-last"))
	require.Equal(t, []string{"b-first", "b-last"}, collect(busB, b.SessionID(), "b-last"))
}
