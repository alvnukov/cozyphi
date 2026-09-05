package editor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestSelectorAttentionSelectionAndSmallOverlayNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	buses := []*controller.Bus{controller.NewBus(nil), controller.NewBus(nil)}
	var views []*sessions.View
	var ids []string
	for i, name := range []string{"main", "review"} {
		v := sessions.NewView(
			application,
			buses[i],
			nil,
			nil,
			nil,
			components.DefaultTheme(),
			t.TempDir(),
			"test",
			"",
			1000,
			nil,
			nil,
		)
		id, err := registry.Open(name, v)
		require.NoError(t, err)
		views = append(views, v)
		ids = append(ids, id)
	}
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	require.Contains(t, drawText(shell), "● main")
	require.Contains(t, drawText(shell), "○ review")
	focus := application.Focused()
	reply := make(chan controller.AskReply, 1)
	buses[1].Publish(controller.PermissionAskMsg{Request: permission.Request{Tool: "bash"}, Reply: reply})
	shell.DrainNow()
	require.Same(t, focus, application.Focused())
	text := drawText(shell)
	require.Contains(t, text, "waiting:permission")
	require.Contains(t, text, "#2 review: bash waiting for permission")
	require.Contains(t, text, "/switch 2")
	require.Contains(t, text, "Ctrl+F10 next")
	require.NotContains(t, text, "Alt+2")
	require.NotEmpty(t, views[1].Status().Attention)
	require.Zero(t, views[1].Status().Unread, "an ask is not a completed turn")
	// Exercise the actual painted selector hit target, not a private formatter.
	surf := shell.Draw(components.DrawContext{Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode})
	var click func(components.Surface) bool
	click = func(s components.Surface) bool {
		if s.Widget != nil && len(s.Buffer) > 0 && strings.Contains(components.SurfaceText(s), "○ review") {
			ctx := &components.EventContext{}
			s.Widget.Handle(ctx, xui.MouseEvent{Button: xui.MouseLeft, Action: xui.MousePress})
			return ctx.Consume
		}
		for _, child := range s.Children {
			if click(child.Surface) {
				return true
			}
		}
		return false
	}
	require.True(t, click(surf))
	active, _ := registry.Active()
	require.Equal(t, ids[1], active.ID)
	require.NotEmpty(t, views[1].Status().Attention, "selection alone is not viewing")
	require.Contains(t, drawText(shell), "● review")
	require.Zero(t, views[1].Status().Unread)
	require.Empty(t, reply, "selection never answers permission")
	narrow := shell.Draw(components.DrawContext{Max: components.Size{Width: 24, Height: 8}, Method: xui.WidthUnicode})
	require.Contains(t, components.SurfaceText(narrow), "● review")
	ctx := &components.EventContext{}
	shell.Capture(ctx, xui.KeyEvent{Code: xui.KeyF10, Mods: xui.ModAlt, Press: true})
	require.True(t, ctx.Consume)
	active, _ = registry.Active()
	require.Equal(t, ids[0], active.ID)
}
