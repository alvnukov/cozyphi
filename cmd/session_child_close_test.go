package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestChildTabClosePreservesOutcomeAndCannotResurrect(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(fmt.Sprintf("running=%t", running), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("COZYPHI_MODEL", "test-model")
			t.Setenv("COZYPHI_API_KEY", "test-key")
			var parentRequests, childRequests atomic.Int32
			childStarted := make(chan struct{})
			markStarted := sync.OnceFunc(func() { close(childStarted) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Tools []struct {
						Function struct{ Name string }
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				parent := false
				for _, tool := range request.Tools {
					parent = parent || tool.Function.Name == "agent_spawn"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if parent && parentRequests.Add(1) == 1 {
					args := `{"prompt":"return the child answer","description":"worker","skills":[],"no_skill_reason":"isolated lifecycle test"}`
					_, _ = fmt.Fprintf(
						w,
						"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"spawn-child\",\"type\":\"function\",\"function\":{\"name\":\"agent_spawn\",\"arguments\":%q}}]}}]}\n\n",
						args,
					)
				} else if parent {
					_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"parent remains\"}}]}\n\n")
				} else {
					if running && childRequests.Add(1) > 1 {
						markStarted()
						_, _ = fmt.Fprint(
							w,
							"data: {\"choices\":[{\"delta\":{\"content\":\"partial child answer\"}}]}\n\n",
						)
						w.(http.Flusher).Flush()
						<-r.Context().Done()
						return
					}
					_, _ = fmt.Fprint(
						w,
						"data: {\"choices\":[{\"delta\":{\"content\":\"preserved child answer\"}}]}\n\n",
					)
				}
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			t.Setenv("COZYPHI_BASE_URL", server.URL)
			cwd, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			proj, err := project.Discover(cwd)
			require.NoError(t, err)
			process, err := controller.NewRuntime(proj)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, process.Close()) })
			process.EnableInteractiveChildren()
			workspace, err := process.Workspace(cwd)
			require.NoError(t, err)
			bus := controller.NewBus(nil)
			parent, err := process.NewSession(bus, workspace, "", nil)
			require.NoError(t, err)
			parent.SetAllowAll(true)
			parent.SetMode(agent.ModeBuild)
			parent.SetAgentsEnabled(true)
			application := app.NewApp(nil)
			registry := sessions.NewRegistry(2, nil)
			ui := editor.NewEditor(application, registry)
			t.Cleanup(func() { require.NoError(t, ui.Close(context.WithoutCancel(t.Context()))) })
			makeView := func(ctrl *controller.Controller, bus *controller.Bus) *sessions.View {
				return sessions.NewView(application, bus, ctrl, commands.NewBuiltinRegistry(), nil,
					components.DefaultTheme(), cwd, ctrl.ModelLabel(), "", 1000, ctrl.ModelNames(), nil)
			}
			parentID, err := registry.Open("main", makeView(parent, bus))
			require.NoError(t, err)
			require.NoError(t, ui.Activate(parentID))
			var child controller.ChildSession
			var childID string
			var stale []controller.ChildSession
			retained := 0
			ui.SetSessionSync(newChildSessionSync(func() []controller.ChildSession {
				if stale != nil {
					return stale
				}
				return process.Children()
			}, func(next controller.ChildSession) {
				retained++
				child = next
				childID, err = registry.Open(next.Name, makeView(next.Controller, next.Bus))
				next.Ready(err)
				require.NoError(t, err)
			}))
			pumpUntil := func(condition func() bool) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for !condition() && time.Now().Before(deadline) {
					ui.DrainNow()
					time.Sleep(time.Millisecond)
				}
				require.True(t, condition(), "lifecycle transition did not complete (%d parent requests): %s",
					parentRequests.Load(), components.SurfaceText(ui.Draw(components.DrawContext{
						Max: components.Size{Width: 160, Height: 60}, Method: xui.WidthUnicode,
					})))
			}
			parent.StartPrompt("launch a child", nil, "")
			pumpUntil(func() bool { return childID != "" })
			pumpUntil(func() bool { return child.Controller.Assignment().Terminal && !child.Controller.RunActive() })
			if running {
				child.Controller.StartPrompt("continue the retained child", nil, "")
				pumpUntil(func() bool {
					select {
					case <-childStarted:
						return true
					default:
						return false
					}
				})
			}
			path := child.Controller.SessionFile()
			requireSessionBusy(t, path)
			stale = []controller.ChildSession{child}
			require.NoError(t, ui.RequestClose(childID))
			if running {
				require.Contains(t, components.SurfaceText(ui.Draw(components.DrawContext{
					Max: components.Size{Width: 100, Height: 25}, Method: xui.WidthUnicode,
				})), "Stop and close worker")
				ui.Capture(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})
				require.True(t, child.Controller.RunActive(), "cancel must leave the child running")
				require.NoError(t, ui.RequestClose(childID))
				ui.Capture(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y', Press: true})
			}
			pumpUntil(func() bool { return registry.Len() == 1 })
			require.Empty(t, process.Children())
			requireSessionFree(t, path)
			metaPath := filepath.Join(proj.JobsDir(), child.JobID, "meta.json")
			data, err := os.ReadFile(metaPath)
			require.NoError(t, err)
			var meta job.Meta
			require.NoError(t, json.Unmarshal(data, &meta))
			require.True(t, meta.Status.Terminal())
			require.NotEmpty(t, meta.OutcomeID, "close must preserve publication of the terminal outcome")
			require.Equal(t, job.StatusCompleted, meta.Status)
			result, readErr := os.ReadFile(meta.ResultPath)
			require.NoError(t, readErr)
			require.Contains(t, string(result), "preserved child answer")
			if running {
				followupID := child.Controller.Assignment().JobID
				require.NotEqual(t, child.JobID, followupID)
				data, err = os.ReadFile(filepath.Join(proj.JobsDir(), followupID, "meta.json"))
				require.NoError(t, err)
				var followup job.Meta
				require.NoError(t, json.Unmarshal(data, &followup))
				require.True(t, followup.Status.Terminal())
				require.NotEmpty(t, followup.OutcomeID)
			}
			// Reconciliation can see an older creation snapshot after closure.
			for range 10 {
				ui.DrainNow()
			}
			require.Equal(t, 1, retained, "a stale creation snapshot cannot resurrect a closed View")
			require.Equal(t, 1, registry.Len())
			active, _ := registry.Active()
			require.Equal(t, parentID, active.ID)
		})
	}
}
