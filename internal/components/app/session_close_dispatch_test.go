package app_test

import (
	"context"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestPaletteCloseDispatchTypesInSurvivor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(3, nil)
	shell := editor.NewEditor(application, registry)
	application.SetRoot(shell)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	cmds := commands.NewBuiltinRegistry()
	cmds.Register(commands.Command{Name: "close", PaletteRoot: func(commands.CommandContext) palette.PaletteCommand {
		return palette.PaletteCommand{ID: "session-close", Noun: "session", Verb: "close tab", Run: func() {
			require.NoError(t, shell.CloseCurrent())
		}}
	}})
	var ids []string
	for _, name := range []string{"closing", "survivor"} {
		view := sessions.NewView(application, controller.NewBus(nil), nil, cmds, nil,
			components.DefaultTheme(), t.TempDir(), "test", "", 1000, nil, nil)
		id, err := registry.Open(name, view)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	require.NoError(t, shell.Activate(ids[0]))
	application.DispatchEvent(xui.KeyEvent{Code: xui.KeyRune, Rune: 'k', Mods: xui.ModCtrl, Press: true})
	for _, r := range "close tab" {
		application.DispatchEvent(xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
	application.DispatchEvent(xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	active, ok := registry.Active()
	require.True(t, ok)
	require.Equal(t, ids[1], active.ID)
	for _, r := range "survivor draft" {
		application.DispatchEvent(xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
	require.Contains(t, components.SurfaceText(shell.Draw(components.DrawContext{
		Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode,
	})), "survivor draft")
}
