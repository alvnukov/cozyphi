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
	"github.com/stretchr/testify/assert"
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

// TestChildLivesInItsParentsFamilyAndCannotResurrect follows one sub-agent
// through the whole shell lifecycle: it never becomes a tab, it is adopted by
// the family of the session that spawned it, its outcome survives the release,
// and a creation snapshot older than that release cannot bring it back.
func TestChildLivesInItsParentsFamilyAndCannotResurrect(t *testing.T) {
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
				view := sessions.NewView(application, bus, ctrl, commands.NewBuiltinRegistry(), nil,
					components.DefaultTheme(), cwd, ctrl.ModelLabel(), "", 1000, ctrl.ModelNames(), nil)
				bindFamilyScreen(ui, view)
				return view
			}
			parentView := makeView(parent, bus)
			parentID, err := registry.Open("main", parentView)
			require.NoError(t, err)
			require.NoError(t, ui.Activate(parentID))
			var child controller.ChildSession
			var snapshot []controller.ChildSession
			var released bool
			built := 0
			ui.SetSessionSync(newChildSessionSync(&childFamilies{
				children: func() []controller.ChildSession {
					if released {
						return snapshot
					}
					return process.Children()
				},
				views: ui.Views,
				build: func(next controller.ChildSession) *sessions.View {
					built++
					child = next
					return makeView(next.Controller, next.Bus)
				},
				retire: ui.RetireChild,
				report: func(msg string) { t.Errorf("unexpected sub-agent report: %s", msg) },
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
			pumpUntil(func() bool { return parentView.Family().Len() == 1 })
			require.True(t, parentView.Family().Has(child.JobID), "the child belongs to the family that spawned it")
			require.Equal(t, 1, registry.Len(), "a sub-agent never becomes a tab")
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

			// The runtime stops retaining the child: the shell releases the row
			// and disposes of the view it built, without touching the selector.
			released, snapshot = true, nil
			pumpUntil(func() bool { return parentView.Family().Len() == 0 })
			pumpUntil(func() bool { return len(process.Children()) == 0 })
			requireSessionFree(t, path)
			require.Equal(t, 1, registry.Len())

			metaPath := filepath.Join(proj.JobsDir(), child.JobID, "meta.json")
			data, err := os.ReadFile(metaPath)
			require.NoError(t, err)
			var meta job.Meta
			require.NoError(t, json.Unmarshal(data, &meta))
			require.True(t, meta.Status.Terminal())
			require.NotEmpty(t, meta.OutcomeID, "release must preserve publication of the terminal outcome")
			require.Equal(t, job.StatusCompleted, meta.Status)
			result, readErr := os.ReadFile(meta.ResultPath)
			require.NoError(t, readErr)
			require.Contains(t, string(result), "preserved child answer")
			if running {
				followupID := child.Controller.Assignment().JobID
				require.NotEqual(t, child.JobID, followupID)
				// The canceled follow-up unwinds on its own goroutine, so its
				// meta lands a moment after the released view is gone. Polling
				// stays on this goroutine: pumpUntil, not require.Eventually.
				followupPath := filepath.Join(proj.JobsDir(), followupID, "meta.json")
				var followup job.Meta
				pumpUntil(func() bool {
					raw, metaErr := os.ReadFile(followupPath)
					if metaErr != nil {
						return false
					}
					followup = job.Meta{}
					return json.Unmarshal(raw, &followup) == nil && followup.Status.Terminal()
				})
				require.NotEmpty(t, followup.OutcomeID)
			}

			// Reconciliation can see a creation snapshot older than the release.
			snapshot = []controller.ChildSession{child}
			for range 10 {
				ui.DrainNow()
			}
			require.Equal(t, 1, built, "a stale creation snapshot cannot resurrect a released sub-agent")
			require.Equal(t, 0, parentView.Family().Len())
			require.Equal(t, 1, registry.Len())
			active, _ := registry.Active()
			require.Equal(t, parentID, active.ID)
		})
	}
}

// A release the user did not ask for takes the screen they are on away from
// them. The shell puts the parent back and says whose screen just went — but
// only when the user was actually looking at that sub-agent.
func TestReleasingTheSubAgentOnScreenSaysWhoseItWas(t *testing.T) {
	for _, tc := range []struct {
		name     string
		onScreen bool
		want     []string
	}{
		{
			name: "the user was on the child's screen", onScreen: true,
			want: []string{"Sub-agent explore(read the loader) released"},
		},
		{name: "the user was on the parent's screen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			application := app.NewApp(nil)
			registry := sessions.NewRegistry(2, nil)
			ui := editor.NewEditor(application, registry)
			t.Cleanup(func() { require.NoError(t, ui.Close(context.WithoutCancel(t.Context()))) })
			makeView := func() *sessions.View {
				view := sessions.NewView(application, controller.NewBus(nil), nil, nil, nil,
					components.DefaultTheme(), t.TempDir(), "test", "", 1000, nil, nil)
				bindFamilyScreen(ui, view)
				return view
			}
			parentView := makeView()
			parentID, err := registry.Open("main", parentView)
			require.NoError(t, err)
			require.NoError(t, ui.Activate(parentID))

			childView := makeView()
			live := []controller.ChildSession{{
				JobID: "job-1", Title: "explore(read the loader)",
				ParentSessionID: parentView.SessionID(), Ready: func(error) {},
			}}
			var notes []string
			sync := newChildSessionSync(&childFamilies{
				children: func() []controller.ChildSession { return live },
				views:    ui.Views,
				build:    func(controller.ChildSession) *sessions.View { return childView },
				retire:   ui.RetireChild,
				report:   func(msg string) { t.Errorf("unexpected sub-agent report: %s", msg) },
				notify:   func(msg string) { notes = append(notes, msg) },
			})

			sync()
			require.Equal(t, 1, parentView.Family().Len(), "the child is held by the session that spawned it")
			if tc.onScreen {
				parentView.Family().OpenAgent("job-1")
				require.Same(t, childView, ui.Screen(), "opening a row is what puts the child on screen")
			}

			live = nil
			sync()
			assert.Equal(t, 0, parentView.Family().Len(), "the runtime released it, so the family lets go")
			assert.Same(t, parentView, ui.Screen(), "and the session that owns it is back on screen")
			assert.Equal(t, tc.want, notes)
		})
	}
}
