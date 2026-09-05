package sessions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/submit"
)

func TestViewCloseRetainsHistoryUntilShellPublication(t *testing.T) {
	testCloseRetainsHistoryUntilShellPublication(t, "view")
}

func TestControllerCloseRetainsHistoryUntilShellPublication(t *testing.T) {
	testCloseRetainsHistoryUntilShellPublication(t, "controller")
}

func TestRuntimeCloseRetainsHistoryUntilShellPublication(t *testing.T) {
	testCloseRetainsHistoryUntilShellPublication(t, "runtime")
}

func testCloseRetainsHistoryUntilShellPublication(t *testing.T, route string) {
	t.Helper()
	for _, tc := range []struct {
		name, command string
		status        session.ToolStatus
	}{
		{name: "success", command: "!exit 0", status: session.ToolDone},
		{name: "failure", command: "!exit 7", status: session.ToolError},
		{name: "canceled", command: "!sleep 30", status: session.ToolCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("COZYPHI_MODEL", "test-model")
			t.Setenv("COZYPHI_API_KEY", "test-key")
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			t.Cleanup(server.Close)
			t.Setenv("COZYPHI_BASE_URL", server.URL)
			cwd := t.TempDir()
			proj, err := project.Discover(cwd)
			require.NoError(t, err)
			runtime, err := controller.NewRuntime(proj)
			require.NoError(t, err)
			t.Cleanup(runtime.Close)
			workspace, err := runtime.Workspace(cwd)
			require.NoError(t, err)
			path := filepath.Join(home, "history.jsonl")
			content := `{"type":"EntrySession","id":"shell-owner","timestamp":"2026-08-23T12:00:00Z","cwd":"/tmp"}`
			require.NoError(t, os.WriteFile(path, []byte(content+"\n"), 0o600))
			bus := controller.NewBus(nil)
			ctrl, err := runtime.NewSession(bus, workspace, path, nil)
			require.NoError(t, err)
			t.Cleanup(ctrl.Close)
			view := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
			constructedRunner := view.bashRunner

			ready, release, published := make(chan session.ToolRun, 1), make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			runner, err := submit.NewBashRunnerInDir(nil, nil, nil, func(msg controller.Msg) {
				ev, ok := msg.(controller.SessionEventMsg)
				if !ok {
					return
				}
				data, ok := ev.Event.(session.ToolData)
				if !ok || data.Run.Status == session.ToolInProgress {
					return
				}
				ready <- data.Run
				// Final publication may depend on controller cancellation. Joining
				// the shell must not postpone that cancellation until publication.
				<-canceled
				<-release
				bus.Publish(msg)
				close(published)
			}, cwd)
			require.NoError(t, err)
			view.bindBashLifetime(runner)
			t.Cleanup(func() {
				ctrl.Cancel()
				unblock()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, view.Close(ctx))
			})
			ctrl.StartPrompt("wait for cancellation", nil, "prompt")
			awaitDisposalSignal(t, started)
			require.True(t, runner.HandleSubmit(tc.command))
			awaitResult := func() {
				t.Helper()
				select {
				case run := <-ready:
					require.Equal(t, tc.status, run.Status)
				case <-time.After(5 * time.Second):
					t.Fatal("shell did not reach final publication")
				}
			}
			if tc.status != session.ToolCancelled {
				awaitResult()
			}
			if route == "view" {
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
				defer cancel()
				require.ErrorIs(t, view.Close(ctx), context.DeadlineExceeded)
			} else {
				returned := make(chan struct{})
				go func() {
					defer close(returned)
					if route == "controller" {
						ctrl.Close()
					} else {
						runtime.Close()
					}
				}()
				// Cancellation must precede the bounded caller's return, even
				// while the publisher refuses to finish until we release it.
				awaitDisposalSignal(t, canceled)
				select {
				case <-returned:
					t.Fatal("close returned before its completion budget expired")
				default:
				}
				awaitDisposalSignal(t, returned)
			}
			awaitDisposalSignal(t, canceled)
			if tc.status == session.ToolCancelled {
				awaitResult()
			}
			require.True(t, runner.Running(), "deadline must not declare publication complete")
			// Let the independently canceled controller finish its own workers.
			require.Eventually(t, func() bool { return !ctrl.RunActive() }, 5*time.Second, time.Millisecond)
			require.Never(t, func() bool {
				owner, openErr := session.OpenSession(path)
				if owner != nil {
					require.NoError(t, owner.Close())
				}
				require.True(t, openErr == nil || errors.Is(openErr, session.ErrBusy))
				return openErr == nil
			}, 50*time.Millisecond, time.Millisecond, "history released before final shell publication")

			unblock()
			awaitDisposalSignal(t, published)
			if route == "view" {
				awaitDisposalSignal(t, view.lifetime.closeDone)
			}
			// No retry is responsible for cleanup: the timed-out disposal owns it.
			require.Eventually(t, func() bool {
				owner, openErr := session.OpenSession(path)
				if owner != nil {
					require.NoError(t, owner.Close())
				}
				require.True(t, openErr == nil || errors.Is(openErr, session.ErrBusy))
				return openErr == nil
			}, 5*time.Second, time.Millisecond)
			require.False(t, runner.Running())
			require.True(t, runner.HandleSubmit("!exit 0"))
			require.False(t, runner.Running(), "disposal must permanently deny shell admission")
			require.True(t, constructedRunner.HandleSubmit("!sleep 30"))
			require.False(t, constructedRunner.Running(), "NewView must register its runner")
			require.NoError(t, view.Close(context.Background()))

			late := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
			require.True(t, late.bashRunner.HandleSubmit("!sleep 30"))
			require.False(t, late.bashRunner.Running(), "late registration must fail closed before exposure")
			require.NoError(t, late.Close(context.Background()))
		})
	}
}

func awaitDisposalSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("disposal barrier was not reached")
	}
}
