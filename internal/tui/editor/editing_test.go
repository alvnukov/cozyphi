package editor

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func TestEditingProfilePersistsAndKeepsDraft(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	home, cwd := t.TempDir(), t.TempDir()
	e := newTestEditorAt(t, home, cwd)
	require.Equal(t, editmode.Standard, e.composer.Chat.EditingMode())
	e.composer.Chat.Value, e.composer.Chat.Cursor = "keep this draft", 4
	require.NoError(t, e.applyEditingMode(editmode.Readline))
	require.Equal(t, "keep this draft", e.composer.Chat.Value)
	require.Equal(t, 4, e.composer.Chat.Cursor)
	mode, err := e.ctrl.EditingMode()
	require.NoError(t, err)
	require.Equal(t, editmode.Readline, mode)
	reopened := newTestEditorAt(t, home, cwd)
	require.Equal(t, editmode.Readline, reopened.composer.Chat.EditingMode())
	require.Equal(t, "F2", keys.Label(keys.CmdPalette))
	require.NoError(t, reopened.applyEditingMode(editmode.Standard))
}

func TestEditingProfileRejectsConflictBeforeSaving(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	e := newTestEditor(t)
	require.NoError(t, keys.Rebind(map[string]string{"plan-editor": "Ctrl+B"}))
	require.ErrorContains(t, e.applyEditingMode(editmode.Readline), "reserved")
	mode, err := e.ctrl.EditingMode()
	require.NoError(t, err)
	require.Equal(t, editmode.Standard, mode)
	require.Equal(t, editmode.Standard, e.composer.Chat.EditingMode())
}

func TestEditingProfileSlashAndPalette(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	e := newTestEditor(t)
	require.True(t, e.commands.DispatchSlash("/keymap vim", e.commandContext()))
	require.Equal(t, editmode.Vim, e.composer.Chat.EditingMode())
	require.True(t, e.commands.DispatchSlash("/keymap invalid", e.commandContext()))
	require.Equal(t, editmode.Vim, e.composer.Chat.EditingMode())
	choices := e.editingChoices()
	require.Len(t, choices, 3)
	require.Contains(t, choices[2].Verb, "(active)")
	choices[1].Run()
	require.Equal(t, editmode.Readline, e.composer.Chat.EditingMode())
}

func TestEditingProfileEventRouting(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	e := newTestEditor(t)
	e.App = app.NewApp(nil)
	e.Focus(&e.composer.Chat)
	dispatch := func(key xui.KeyEvent) {
		ctx := &components.EventContext{DeliveredTo: e.App.Focused()}
		if ctx.DeliveredTo != nil && ctx.DeliveredTo != e {
			ctx.DeliveredTo.Handle(ctx, key)
		}
		if !ctx.Consume {
			e.Handle(ctx, key)
		}
		if ctx.Focus != nil {
			e.App.RequestFocus(ctx.Focus)
		}
	}
	require.NoError(t, e.applyEditingMode(editmode.Readline))
	e.composer.Chat.Value, e.composer.Chat.Cursor = "hello world", 5
	dispatch(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'k', Mods: xui.ModCtrl})
	require.Equal(t, "hello", e.composer.Chat.Value, "Ctrl+K edits, not palette")
	require.Same(t, &e.composer.Chat, e.App.Focused())
	dispatch(xui.KeyEvent{Press: true, Code: xui.KeyF2})
	require.NotSame(t, &e.composer.Chat, e.App.Focused(), "F2 opens palette")
	dispatch(xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	require.Same(t, &e.composer.Chat, e.App.Focused())
	require.NoError(t, e.applyEditingMode(editmode.Vim))
	dispatch(xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	require.Equal(t, "VIM NORMAL", e.composer.Chat.EditingLabel())
	require.Equal(t, "hello", e.composer.Chat.Value)
	dispatch(xui.KeyEvent{Press: true, Code: xui.KeyF1})
	require.True(t, e.help.Visible(), "global help remains available in NORMAL")
}
