package sessions

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

func TestKeymapShortcutCyclesAndPersists(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	home, cwd := t.TempDir(), t.TempDir()
	e := newTestEditorAt(t, home, cwd)
	e.App = app.NewApp(nil)
	e.Focus(&e.composer.Chat)
	e.composer.Chat.Value, e.composer.Chat.Cursor = "keep this draft", 4
	for _, mode := range []editmode.Mode{editmode.Readline, editmode.Vim, editmode.Standard} {
		cursor := e.composer.Chat.Cursor
		dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF6})
		require.Equal(t, mode, e.composer.Chat.EditingMode())
		require.Equal(t, "keep this draft", e.composer.Chat.Value)
		require.Equal(t, cursor, e.composer.Chat.Cursor)
		require.Same(t, &e.composer.Chat, e.App.Focused())
		saved, err := e.ctrl.EditingMode()
		require.NoError(t, err)
		require.Equal(t, mode, saved)
		reopened := newTestEditorAt(t, home, cwd)
		require.Equal(t, mode, reopened.composer.Chat.EditingMode())
		if mode == editmode.Vim {
			require.Equal(t, "VIM INSERT", e.composer.Chat.EditingLabel())
			require.Equal(t, "VIM INSERT", reopened.composer.Chat.EditingLabel())
			dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
			require.Equal(t, "VIM NORMAL", e.composer.Chat.EditingLabel())
		}
	}
}

func TestKeymapShortcutUsesRebindingAndRejectsConflict(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	e := newTestEditor(t)
	e.App = app.NewApp(nil)
	e.Focus(&e.composer.Chat)
	e.composer.Chat.Value, e.composer.Chat.Cursor = "keep draft", 4
	require.NoError(t, keys.Rebind(map[string]string{"keymap": "F9", "plan-editor": "Ctrl+B"}))
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF6})
	require.Equal(t, editmode.Standard, e.composer.Chat.EditingMode())
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF9})
	require.Equal(t, editmode.Standard, e.composer.Chat.EditingMode(), "conflicting Readline profile is rejected")
	saved, err := e.ctrl.EditingMode()
	require.NoError(t, err)
	require.Equal(t, editmode.Standard, saved)
	require.Equal(t, "keep draft", e.composer.Chat.Value)
	require.Equal(t, 4, e.composer.Chat.Cursor)
	require.NoError(t, keys.Rebind(map[string]string{"keymap": "F9"}))
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF9})
	require.Equal(t, editmode.Readline, e.composer.Chat.EditingMode())
	require.Equal(t, "F9", keys.Label(keys.CmdKeymap))
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF6, Mods: xui.ModShift})
	require.Equal(t, editmode.Readline, e.composer.Chat.EditingMode(), "verbose shortcut must not cycle")
	require.Equal(t, "keep draft", e.composer.Chat.Value)
	require.Equal(t, 4, e.composer.Chat.Cursor)
	dispatchControlKey(e, xui.KeyEvent{Press: false, Code: xui.KeyF9})
	require.Equal(t, editmode.Readline, e.composer.Chat.EditingMode(), "releases must not cycle")
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF1})
	require.True(t, e.help.Visible())
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF9})
	require.Equal(t, editmode.Readline, e.composer.Chat.EditingMode(), "help owns keys while open")
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF9})
	require.Equal(t, editmode.Vim, e.composer.Chat.EditingMode())
	require.Equal(t, "VIM INSERT", e.composer.Chat.EditingLabel())
	require.Equal(t, "keep draft", e.composer.Chat.Value)
	require.Equal(t, 4, e.composer.Chat.Cursor)
}
