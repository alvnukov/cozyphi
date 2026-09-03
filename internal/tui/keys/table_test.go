package keys

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreDefaults(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		require.NoError(t, Rebind(nil))
	})
}

func TestDefaultTableBindsEveryCommand(t *testing.T) {
	require.NoError(t, Rebind(nil))
	for _, cmd := range commands {
		assert.NotEmpty(t, Label(cmd), "command %q has no default chord", cmd)
	}
}

// TestRebindChangesDispatchAndEveryDisplay is the override contract: one
// Rebind moves the behavior (GlobalCommand), the palette label (Label), the
// footer (Hints) and the help screen (Find) together.
func TestRebindChangesDispatchAndEveryDisplay(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{
		"help":           "F2",
		"sidebar-toggle": "Ctrl+B",
	}))

	cmd, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF2})
	require.True(t, ok)
	assert.Equal(t, CmdHelp, cmd, "the new chord dispatches")
	_, ok = GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyF1})
	assert.False(t, ok, "the old chord is gone")

	assert.Equal(t, "F2", Label(CmdHelp), "the palette shortcut follows")
	hints := Hints(ScopeSidebar)
	assert.Contains(t, hints, "Ctrl+B hide", "the footer follows")
	assert.NotContains(t, hints, "Ctrl+O")

	global, found := Find(ScopeGlobal)
	require.True(t, found)
	var helpKeys []string
	for _, b := range global.Bindings {
		if b.Cmd == CmdHelp {
			helpKeys = b.Keys
		}
	}
	assert.Equal(t, []string{"F2"}, helpKeys, "the help screen follows")
}

// TestRebindRefusesTwoCommandsOnOneChord is the conflict criterion: a chord
// may own one command per scope, and the error names all three parties.
func TestRebindRefusesTwoCommandsOnOneChord(t *testing.T) {
	restoreDefaults(t)
	err := Rebind(map[string]string{"help": "Ctrl+K"})
	require.Error(t, err)
	for _, part := range []string{"Ctrl+K", "help", "palette"} {
		assert.Contains(t, err.Error(), part)
	}
	assert.Equal(t, "F1", Label(CmdHelp), "a refused rebind leaves the table untouched")
}

func TestRebindRefusesNonsense(t *testing.T) {
	restoreDefaults(t)
	err := Rebind(map[string]string{"warp-core": "F2"})
	require.ErrorContains(t, err, `unknown command "warp-core"`)

	err = Rebind(map[string]string{"help": "Meta+Q"})
	require.ErrorContains(t, err, "help")
	require.ErrorContains(t, err, "Meta")
}

// TestNoneUnbindsACommand: "none" frees the chord and removes the rows that
// would otherwise advertise a key that no longer works.
func TestNoneUnbindsACommand(t *testing.T) {
	restoreDefaults(t)
	require.NoError(t, Rebind(map[string]string{"plan-details": "none"}))

	_, ok := GlobalCommand(xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'd', Mods: xui.ModCtrl})
	assert.False(t, ok)
	assert.Empty(t, Label(CmdPlanDetails))

	g, found := Find(ScopeSidebar)
	require.True(t, found)
	for _, b := range g.Bindings {
		assert.NotEqual(t, CmdPlanDetails, b.Cmd, "an unbound command has no row to show")
	}
}

func TestSettingsTitleNamesTheCurrentChord(t *testing.T) {
	restoreDefaults(t)
	g, ok := Find(ScopeSettings)
	require.True(t, ok)
	assert.Equal(t, "Settings (Ctrl+,)", g.Title)

	require.NoError(t, Rebind(map[string]string{"settings": "F9"}))
	g, ok = Find(ScopeSettings)
	require.True(t, ok)
	assert.Equal(t, "Settings (F9)", g.Title)
}

// TestCopyLastKeepsBothSpellings: a comma-separated default is two
// interchangeable chords for one command, rendered as one catalog row.
func TestCopyLastKeepsBothSpellings(t *testing.T) {
	require.NoError(t, Rebind(nil))
	for _, ev := range []xui.KeyEvent{
		{Press: true, Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl | xui.ModShift},
		{Press: true, Code: xui.KeyRune, Rune: 'c', Mods: xui.ModSuper},
	} {
		cmd, ok := GlobalCommand(ev)
		require.True(t, ok)
		assert.Equal(t, CmdCopyLast, cmd)
	}
	g, ok := Find(ScopeTranscript)
	require.True(t, ok)
	joined := ""
	for _, b := range g.Bindings {
		if b.Cmd == CmdCopyLast {
			joined = strings.Join(b.Keys, "/")
		}
	}
	assert.Equal(t, "Ctrl+Shift+C/Cmd+C", joined)
}
