package controller_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestRuntimeWorkspacesKeepCanonicalDirectoryIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	rt, err := controller.NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close()) })

	first, err := rt.Workspace(cwd)
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(cwd, alias))
	same, err := rt.Workspace(alias)
	require.NoError(t, err)
	require.Same(t, first, same, "aliases share resources for the same canonical cwd")

	other, err := rt.Workspace(t.TempDir())
	require.NoError(t, err)
	require.NotSame(t, first, other, "different workdirs must never share hooks/MCP/LSP identity")
	require.NoError(t, rt.Close())
	_, err = rt.Workspace(cwd)
	require.ErrorContains(t, err, "closed")
}

func TestRuntimeSessionsKeepStateAndCloseIndependent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	rt, err := controller.NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close()) })
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	first, err := rt.NewSession(controller.NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	second, err := rt.NewSession(controller.NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.NotEqual(t, first.SessionID(), second.SessionID())
	first.SetAllowAll(true)
	require.False(t, second.AllowAll())
	first.SetPlanEnabled(false)
	require.Empty(t, second.Plan().Items)
	secondID := second.SessionID()
	first.Close()
	first.Close()
	require.Equal(t, secondID, second.SessionID())
	require.NoError(t, second.Clear(), "closing one session must leave its sibling usable")
	third, err := rt.NewSession(controller.NewBus(nil), ws, "", nil)
	require.NoError(t, err, "closing sessions does not shut down the process runtime")
	third.Close()
	require.NoError(t, rt.Close())
	require.Error(t, second.Clear(), "runtime shutdown must close every retained session")
	_, err = rt.NewSession(controller.NewBus(nil), ws, "", nil)
	require.ErrorContains(t, err, "closed")
}

func TestRuntimeClosingOneActiveSessionDoesNotCancelSibling(t *testing.T) {
	type request struct {
		body string
		ctx  context.Context
	}
	requests := make(chan request, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- request{body: string(body), ctx: r.Context()}
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"sibling completed\"}}]}\n\ndata: [DONE]\n\n",
		)
	}))
	t.Cleanup(server.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", server.URL+"/v1")
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	rt, err := controller.NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close()) })
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	first, err := rt.NewSession(controller.NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	bus := controller.NewBus(nil)
	second, err := rt.NewSession(bus, ws, "", nil)
	require.NoError(t, err)
	first.StartPrompt("first isolated prompt", nil, "")
	a := runtimeSignal(t, requests)
	second.StartPrompt("second isolated prompt", nil, "")
	b := runtimeSignal(t, requests)
	require.Contains(t, a.body, "first isolated prompt")
	require.NotContains(t, a.body, "second isolated prompt")
	require.Contains(t, b.body, "second isolated prompt")
	require.NotContains(t, b.body, "first isolated prompt")
	first.Close()
	runtimeSignal(t, a.ctx.Done())
	require.NoError(t, b.ctx.Err())
	require.True(t, second.RunActive())
	close(release)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				if _, ok := msg.(controller.RunEndedMsg); ok {
					require.False(t, second.RunActive())
					return
				}
			}
		case <-deadline:
			t.Fatal("sibling session never finished")
		}
	}
}

func runtimeSignal[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runtime signal")
		var zero T
		return zero
	}
}
