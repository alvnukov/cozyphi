package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/clipboard"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
	"github.com/alvnukov/cozyphi/internal/voice"
)

func TestRetainedUIOwnsAcquiredHistoryUntilDisposal(t *testing.T) {
	for _, args := range [][]string{{"-c"}, {"--resume", "first"}} {
		t.Run(args[0], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("COZYPHI_MODEL", "test-model")
			t.Setenv("COZYPHI_API_KEY", "test-key")
			t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
			cwd, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			proj, err := project.Discover(cwd)
			require.NoError(t, err)
			process, err := controller.NewRuntime(proj)
			require.NoError(t, err)
			t.Cleanup(process.Close)
			workspace, err := process.Workspace(proj.Root())
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(proj.SessionDir(), 0o700))
			firstPath := writeTUISession(t, proj.SessionDir(), "first-owner", time.Now())
			opts, err := parseTUIArgs(args)
			require.NoError(t, err)
			acquired, err := resolveTUIResumeSession(opts, proj.SessionDir())
			require.NoError(t, err)
			require.NotNil(t, acquired)
			t.Cleanup(func() { require.NoError(t, acquired.Close()) })
			requireSessionBusy(t, firstPath)

			application := app.NewApp(nil)
			registry := sessions.NewRegistry(12, nil)
			ui := editor.NewEditor(application, registry)
			t.Cleanup(func() { require.NoError(t, ui.Close(context.WithoutCancel(t.Context()))) })
			hist := history.Open(filepath.Join(home, "history"))
			gate := voice.NewCaptureGate()
			settingsManager, err := harnesssettings.Open(proj.Global().ConfigFile(), process.PlanRuntime(), nil)
			require.NoError(t, err)
			create := func(path string, owner *session.Manager) (*sessions.View, *controller.Controller, string) {
				t.Helper()
				bus := controller.NewBus(nil)
				ctrl, err := process.NewSession(bus, workspace, path, owner)
				require.NoError(t, err, "the initially acquired manager must not be reopened")
				t.Cleanup(ctrl.Close)
				view := newTUIView(application, nil, components.DefaultTheme(), proj, ctrl, bus,
					hist, workspace.Root(), gate, commands.NewBuiltinRegistry(), settingsManager)
				view.SetClipboardReader(noClipboardImage)
				view.ConfigureSessionNavigation(registry, ui.Activate)
				id, err := registry.Open(ctrl.SessionID(), view)
				require.NoError(t, err)
				return view, ctrl, id
			}
			first, firstCtrl, firstID := create(firstPath, acquired)
			require.Equal(t, acquired.ID(), firstCtrl.SessionID())
			require.NoError(t, ui.Activate(firstID))
			ui.Handle(&components.EventContext{}, xui.PasteEvent{Text: "retained draft"})
			secondPath := writeTUISession(t, proj.SessionDir(), "second-owner", time.Now())
			second, secondCtrl, secondID := create(secondPath, nil)
			require.NoError(t, ui.Activate(secondID))
			requireSessionBusy(t, firstPath)
			requireSessionBusy(t, secondPath)

			// Prefix resume navigates to the owner without replacing either graph.
			second.ResumeSession("first")
			active, ok := registry.Active()
			require.True(t, ok)
			require.Equal(t, firstID, active.ID)
			require.True(t, first.Active())
			require.False(t, second.Active())
			require.Equal(t, firstPath, firstCtrl.SessionFile())
			require.Equal(t, secondPath, secondCtrl.SessionFile())
			require.Contains(t, components.SurfaceText(ui.Draw(components.DrawContext{
				Max: components.Size{Width: 120, Height: 30}, Method: xui.WidthUnicode,
			})), "retained draft")
			first.ResumeSession("first-owner") // The selected owner is also a no-op.
			requireSessionBusy(t, firstPath)

			alias := filepath.Join(t.TempDir(), "alias.jsonl")
			require.NoError(t, os.Symlink(firstPath, alias))
			require.NoError(t, ui.Activate(secondID))
			second.ResumeSession(alias)
			active, _ = registry.Active()
			require.Equal(t, firstID, active.ID, "canonical aliases must select the retained owner")

			externalPath := writeTUISession(t, proj.SessionDir(), "first-external", time.Now())
			external, err := session.OpenSession(externalPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, external.Close()) })
			require.NoError(t, ui.Activate(secondID))
			before, err := os.ReadFile(secondPath)
			require.NoError(t, err)
			for _, id := range []string{"first", "first-external", "missing"} {
				second.ResumeSession(id)
				active, _ = registry.Active()
				require.Equal(t, secondID, active.ID, "ambiguous, busy and missing histories cannot change selection")
				require.Equal(t, secondPath, secondCtrl.SessionFile())
				require.Equal(t, firstPath, firstCtrl.SessionFile())
				require.Equal(t, 2, registry.Len())
				after, err := os.ReadFile(secondPath)
				require.NoError(t, err)
				require.Equal(t, before, after)
				requireSessionBusy(t, firstPath)
				requireSessionBusy(t, secondPath)
			}

			// Unretained history still replaces the current conversation once free.
			require.NoError(t, external.Close())
			second.ResumeSession("first-external")
			require.Equal(t, externalPath, secondCtrl.SessionFile())
			requireSessionFree(t, secondPath)
			requireSessionBusy(t, firstPath)
			requireSessionBusy(t, externalPath)

			require.NoError(t, first.Close(context.WithoutCancel(t.Context())))
			requireSessionFree(t, firstPath)
			requireSessionBusy(t, externalPath)
			require.NoError(t, ui.Close(context.WithoutCancel(t.Context())))
			requireSessionFree(t, externalPath)
		})
	}
}

func requireSessionBusy(t *testing.T, path string) {
	t.Helper()
	owner, err := session.OpenSession(path)
	if owner != nil {
		t.Cleanup(func() { require.NoError(t, owner.Close()) })
	}
	require.ErrorIs(t, err, session.ErrBusy)
}

func requireSessionFree(t *testing.T, path string) {
	t.Helper()
	owner, err := session.OpenSession(path)
	require.NoError(t, err)
	require.NoError(t, owner.Close())
}

// noClipboardImage keeps a synthetic PasteEvent a text paste regardless of
// what the developer's clipboard holds.
func noClipboardImage() (clipboard.Image, bool, error) { return clipboard.Image{}, false, nil }
