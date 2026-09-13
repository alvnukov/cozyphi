package keys

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestBackgroundShellBindingFollowsEditingProfile(t *testing.T) {
	restoreDefaults(t)
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Vim} {
		require.NoError(t, SetProfile(mode))
		cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'b', Mods: xui.ModCtrl})
		require.True(t, ok)
		require.Equal(t, CmdBackgroundShell, cmd)
	}
	require.NoError(t, SetProfile(editmode.Readline))
	require.Equal(t, "Shift+F4", Label(CmdBackgroundShell))
	_, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'b', Mods: xui.ModCtrl})
	require.False(t, ok)
	require.NoError(t, Rebind(map[string]string{"background-shell": "F9"}))
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		require.NoError(t, SetProfile(mode))
		require.Equal(t, "F9", Label(CmdBackgroundShell))
	}
}
