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
		CmdPalette: "F2", CmdPlanEditor: "F3", CmdWatches: "F4", CmdVerbose: "F6",
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
