package editor

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// TestEditorDispatchesGlobalChordsThroughTheTable: the editor compares no
// chord itself, so rebinding a command in the keys table moves the behavior
// with it — the default chord stops working and the configured one works.
func TestEditorDispatchesGlobalChordsThroughTheTable(t *testing.T) {
	e := newTestEditor(t)
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	require.NoError(t, keys.Rebind(map[string]string{"plan-editor": "Ctrl+G"}))

	press := func(r rune, mods xui.Modifiers) {
		e.Handle(&components.EventContext{},
			xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r, Mods: mods})
	}
	press('p', xui.ModCtrl)
	assert.False(t, e.planPane.Visible(), "the overridden default chord must not fire")
	press('g', xui.ModCtrl)
	assert.True(t, e.planPane.Visible(), "the configured chord opens the plan editor")
}
