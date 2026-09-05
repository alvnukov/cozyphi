package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
)

func TestRuntimeCloseCancelsPendingConstructionWithoutBlockingOwners(t *testing.T) {
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
	t.Cleanup(func() { require.NoError(t, rt.Close()) })
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	first, err := rt.NewSession(NewBus(nil), ws, "")
	require.NoError(t, err)

	running, cancelled := make(chan struct{}), make(chan struct{})
	_, err = rt.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: "live work", ParentID: first.SessionID(),
	}, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		close(running)
		<-ctx.Done()
		close(cancelled)
		return "", ctx.Err()
	}))
	require.NoError(t, err)
	<-running

	entered, release := make(chan struct{}), make(chan struct{})
	ws.hooks = hooks.NewManager(hooks.Entry{
		Kind: hooks.KindSessionStart,
		Hook: hooks.FuncHook{
			HookName: "controlled startup",
			Sess: func(ctx context.Context, _ hooks.SessionEvent) (hooks.SessionResult, error) {
				close(entered)
				select {
				case <-ctx.Done():
					return hooks.SessionResult{}, ctx.Err()
				case <-release:
					return hooks.SessionResult{}, nil
				}
			},
		},
	})
	constructed := make(chan error, 1)
	go func() {
		_, err := rt.NewSession(NewBus(nil), ws, "")
		constructed <- err
	}()
	<-entered
	closed := make(chan struct{})
	go func() { _ = rt.Close(); close(closed) }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Error("startup hook blocked cancellation of already-live work")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("runtime Close waited on startup rather than canceling it")
	}
	close(release)
	select {
	case err := <-constructed:
		require.ErrorContains(t, err, "closed", "a closing runtime must reject a late constructed session")
	case <-time.After(5 * time.Second):
		t.Fatal("constructor did not exit")
	}
	<-closed
}
