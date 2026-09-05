package keys

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestReadlineProfileKeepsCommandsReachable(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, SetProfile(editmode.Readline))
	for cmd, label := range map[Command]string{
		CmdPalette: "F2", CmdPlanEditor: "F3", CmdWatches: "F4", CmdVerbose: "Shift+F6",
	} {
		require.Equal(t, label, Label(cmd))
	}
	for _, key := range "abdefhknpuwy" {
		_, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: key, Mods: xui.ModCtrl})
		require.False(t, ok, "Readline must own Ctrl+%c", key)
	}
	require.NoError(t, SetProfile(editmode.Standard))
	require.Equal(t, "Ctrl+K", Label(CmdPalette))
}

func TestReadlineProfileRejectsConflictWithoutChangingTable(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"plan-editor": "Ctrl+B"}))
	require.ErrorContains(t, SetProfile(editmode.Readline), "reserved by readline")
	require.Equal(t, "Ctrl+K", Label(CmdPalette))
	require.Equal(t, "Ctrl+B", Label(CmdPlanEditor))
}

func TestProfilePreservesCustomBindings(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"palette": "F9"}))
	for _, mode := range []editmode.Mode{editmode.Vim, editmode.Readline, editmode.Standard} {
		require.NoError(t, SetProfile(mode))
		require.Equal(t, "F9", Label(CmdPalette))
	}
}

func TestProfilesRejectInvalidAndUndoBindings(t *testing.T) {
	restoreDefaults(t)
	require.Error(t, SetProfile(editmode.Mode("invalid")))
	require.Equal(t, "Ctrl+K", Label(CmdPalette))
	for _, chord := range []string{"Ctrl+Z", "Ctrl+Y", "Ctrl+Shift+Z"} {
		require.ErrorContains(t, CheckBinds(map[string]string{"help": chord}), "reserved")
	}
}

func TestProfileHelpFollowsSelection(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, SetProfile(editmode.Readline))
	g, ok := Find(ScopeComposer)
	require.True(t, ok)
	found := false
	for _, b := range g.Bindings {
		if b.Label() == "Ctrl+A" {
			require.Contains(t, b.Desc, "beginning")
			found = true
		}
	}
	require.True(t, found)
	require.NoError(t, SetProfile(editmode.Vim))
	g, _ = Find(ScopeComposer)
	require.Contains(t, g.Note, "NORMAL never sends")
}

func TestKeymapBindingProfiles(t *testing.T) {
	restoreDefaults(t)
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		require.NoError(t, SetProfile(mode))
		cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF6})
		require.True(t, ok)
		require.Equal(t, CmdKeymap, cmd)
	}
	require.NoError(t, SetProfile(editmode.Readline))
	cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF6, Mods: xui.ModShift})
	require.True(t, ok)
	require.Equal(t, CmdVerbose, cmd)
}

func TestKeymapRebindingConflictsAndHelp(t *testing.T) {
	restoreDefaults(t)
	require.ErrorContains(t, CheckBinds(map[string]string{"help": "F6"}), "both")
	require.ErrorContains(t, Rebind(map[string]string{"keymap": "F1"}), "both")
	require.Equal(t, "F6", Label(CmdKeymap))
	require.NoError(t, Rebind(map[string]string{"keymap": "F9"}))
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		require.NoError(t, SetProfile(mode))
		require.Equal(t, "F9", Label(CmdKeymap))
		_, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF6})
		require.False(t, ok)
		cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF9})
		require.True(t, ok)
		require.Equal(t, CmdKeymap, cmd)
		g, ok := Find(ScopeGlobal)
		require.True(t, ok)
		found := false
		for _, b := range g.Bindings {
			if b.Cmd != CmdKeymap {
				continue
			}
			found = true
			require.Equal(t, "F9", b.Label())
			require.Contains(t, b.Desc, "standard")
			require.Contains(t, b.Desc, "readline")
			require.Contains(t, b.Desc, "vim")
		}
		require.True(t, found, "keymap cycling is discoverable in help")
	}
	require.NoError(t, Rebind(map[string]string{"keymap": "none"}))
	require.Empty(t, Label(CmdKeymap))
	_, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF6})
	require.False(t, ok)
	g, ok := Find(ScopeGlobal)
	require.True(t, ok)
	for _, b := range g.Bindings {
		require.NotEqual(t, CmdKeymap, b.Cmd)
	}
}
