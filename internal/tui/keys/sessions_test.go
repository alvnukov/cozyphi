package keys_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func TestSessionNavigationStaysAvailableAcrossEditingProfiles(t *testing.T) {
	require.NoError(t, keys.Rebind(nil))
	t.Cleanup(func() {
		require.NoError(t, keys.Rebind(nil))
		require.NoError(t, keys.SetProfile(editmode.Standard))
	})
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		require.NoError(t, keys.SetProfile(mode))
		for _, tc := range []struct {
			mods    xui.Modifiers
			command keys.Command
		}{
			{xui.ModCtrl, keys.CmdSessionNext},
			{xui.ModShift, keys.CmdSessionPrev},
			{xui.ModAlt, keys.CmdSessionBack},
		} {
			command, ok := keys.GlobalCommand(
				xui.KeyEvent{Code: xui.KeyF10, Mods: tc.mods, Press: true},
			)
			require.True(t, ok)
			require.Equal(t, tc.command, command)
		}
	}
	require.NoError(t, keys.Rebind(map[string]string{"session-next": "F9"}))
	require.Equal(t, "F9", keys.Label(keys.CmdSessionNext))
	require.ErrorContains(t, keys.CheckBinds(map[string]string{"help": "Ctrl+F10"}), "both")
	group, ok := keys.Find(keys.ScopeGlobal)
	require.True(t, ok)
	for _, command := range []keys.Command{keys.CmdSessionNext, keys.CmdSessionPrev, keys.CmdSessionBack} {
		found := false
		for _, binding := range group.Bindings {
			found = found || binding.Cmd == command
		}
		require.True(t, found, "session navigation must be discoverable in help: %s", command)
	}
}
