package editor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func closeTestShell(t *testing.T) (*editor.Editor, *sessions.Registry, []string, []*controller.Bus) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(3, nil)
	var ids []string
	var buses []*controller.Bus
	for _, name := range []string{"main", "worker", "survivor"} {
		bus := controller.NewBus(nil)
		view := sessions.NewView(application, bus, nil, nil, nil, components.DefaultTheme(),
			t.TempDir(), "test", "", 1000, nil, nil)
		id, err := registry.Open(name, view)
		require.NoError(t, err)
		ids = append(ids, id)
		buses = append(buses, bus)
	}
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	return shell, registry, ids, buses
}

func awaitTabCount(t *testing.T, shell *editor.Editor, registry *sessions.Registry, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for registry.Len() != count && time.Now().Before(deadline) {
		shell.DrainNow() // UI state stays on this goroutine, including under -race.
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, count, registry.Len())
}

func TestCloseCurrentBackgroundAndCompletedTabs(t *testing.T) {
	for _, target := range []string{"current", "background", "completed"} {
		t.Run(target, func(t *testing.T) {
			shell, registry, ids, buses := closeTestShell(t)
			id := ids[0]
			if target != "current" {
				id = ids[1]
			}
			if target == "completed" {
				buses[1].Publish(controller.SetActivityMsg{Activity: controller.ActivityTools})
				buses[1].Publish(controller.RunEndedMsg{})
			}
			if target == "current" {
				require.NoError(t, shell.CloseCurrent())
			} else {
				require.NoError(t, shell.RequestClose(id))
			}
			require.NotContains(t, drawText(shell), "Stop and close")
			awaitTabCount(t, shell, registry, 2)
			active, ok := registry.Active()
			require.True(t, ok)
			require.NotEqual(t, id, active.ID)
			if target != "current" {
				require.Equal(t, ids[0], active.ID, "background close cannot change selection")
			}
			require.Error(t, shell.Activate(id))
			require.Error(t, shell.RequestClose(id), "stale close cannot affect a replacement")
		})
	}
}

func TestCloseRunningConfirmationCancelAndTargetIdentity(t *testing.T) {
	shell, registry, ids, buses := closeTestShell(t)
	buses[1].Publish(controller.SetActivityMsg{Activity: controller.ActivityTools})
	for _, cancelKey := range []xui.KeyEvent{
		{Code: xui.KeyEscape, Press: true},
		{Code: xui.KeyEnter, Press: true},
		{Code: xui.KeyRune, Rune: 'n', Press: true},
	} {
		require.NoError(t, shell.RequestClose(ids[1]))
		text := drawText(shell)
		require.Contains(t, text, "Stop and close worker")
		require.Contains(t, text, ids[1])
		ctx := &components.EventContext{}
		shell.Capture(ctx, xui.PasteEvent{Text: "y"})
		require.True(t, ctx.Consume)
		require.Contains(t, drawText(shell), "Stop and close worker", "paste is not explicit confirmation")
		shell.Capture(&components.EventContext{}, cancelKey)
		require.NotContains(t, drawText(shell), "Stop and close")
		require.Equal(t, 3, registry.Len())
		require.True(t, registry.Entries()[1].View.Status().Running)
	}
	require.NoError(t, shell.RequestClose(ids[1]))
	require.NoError(t, shell.Activate(ids[2])) // Selection changes cannot retarget the ask.
	shell.Capture(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y', Press: true})
	awaitTabCount(t, shell, registry, 2)
	active, _ := registry.Active()
	require.Equal(t, ids[2], active.ID)
	require.Error(t, shell.Activate(ids[1]))
	require.NoError(t, shell.Activate(ids[0]))
}

func TestCloseRefusesLastSurvivingTab(t *testing.T) {
	shell, registry, ids, _ := closeTestShell(t)
	require.NoError(t, shell.RequestClose(ids[1]))
	require.NoError(t, shell.RequestClose(ids[2]))
	// Both pending closes still occupy registry slots, but neither is a survivor.
	require.ErrorContains(t, shell.CloseCurrent(), "last tab")
	awaitTabCount(t, shell, registry, 1)
	require.ErrorContains(t, shell.CloseCurrent(), "last tab")
	require.True(t, registry.Entries()[0].View.Active())
}

func TestTabCloseHitTargetDoesNotSelectBackgroundTab(t *testing.T) {
	shell, registry, ids, _ := closeTestShell(t)
	surface := shell.Draw(
		components.DrawContext{Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode},
	)
	var targets []components.Widget
	var visit func(components.Surface)
	visit = func(s components.Surface) {
		if s.Widget != nil && len(s.Buffer) > 0 && strings.HasPrefix(components.SurfaceText(s), "×") {
			targets = append(targets, s.Widget)
		}
		for _, child := range s.Children {
			visit(child.Surface)
		}
	}
	visit(surface)
	require.Len(t, targets, 3)
	ctx := &components.EventContext{}
	targets[1].Handle(ctx, xui.MouseEvent{Button: xui.MouseLeft, Action: xui.MousePress})
	require.True(t, ctx.Consume)
	active, _ := registry.Active()
	require.Equal(t, ids[0], active.ID)
	awaitTabCount(t, shell, registry, 2)
	require.Error(t, shell.Activate(ids[1]))
}

func TestNavigationSkipsClosingTabs(t *testing.T) {
	shell, registry, ids, _ := closeTestShell(t)
	require.NoError(t, shell.RequestClose(ids[1]))
	require.ErrorContains(t, shell.Jump(2), "closing")
	shell.Capture(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyF10, Mods: xui.ModCtrl, Press: true})
	active, _ := registry.Active()
	require.Equal(t, ids[2], active.ID, "next skips a retained closing slot")
	shell.Capture(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyF10, Mods: xui.ModAlt, Press: true})
	active, _ = registry.Active()
	require.Equal(t, ids[0], active.ID, "previous skips a retained closing slot")
}

func TestCloseSelectsAdjacentSurvivor(t *testing.T) {
	for _, target := range []int{1, 2} {
		t.Run([]string{"middle", "last"}[target-1], func(t *testing.T) {
			shell, registry, ids, _ := closeTestShell(t)
			require.NoError(t, shell.Activate(ids[target]))
			require.NoError(t, shell.CloseCurrent())
			active, _ := registry.Active()
			want := ids[2]
			if target == 2 {
				want = ids[1]
			}
			require.Equal(t, want, active.ID, "close selects next, or previous at the end")
			awaitTabCount(t, shell, registry, 2)
			active, _ = registry.Active()
			require.Equal(t, want, active.ID, "cleanup completion must not move selection again")
		})
	}
}
