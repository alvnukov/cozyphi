package keys

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestEffortBindingProfilesAndOverride(t *testing.T) {
	restoreDefaults(t)
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		require.NoError(t, SetProfile(mode))
		cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF5})
		require.True(t, ok)
		require.Equal(t, CmdEffort, cmd)
	}
	require.ErrorContains(t, CheckBinds(map[string]string{"help": "F5"}), "both")
	require.NoError(t, Rebind(map[string]string{"effort": "F9"}))
	require.NoError(t, SetProfile(editmode.Readline))
	require.Equal(t, "F9", Label(CmdEffort))
	g, ok := Find(ScopeGlobal)
	require.True(t, ok)
	found := false
	for _, b := range g.Bindings {
		if b.Cmd == CmdEffort {
			found = true
			require.Equal(t, "F9", b.Label())
		}
	}
	require.True(t, found, "effort is discoverable in help")
}
