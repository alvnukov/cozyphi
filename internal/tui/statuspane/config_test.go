package statuspane_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestConfigIsDetachedReadOnlyAndIgnoresEditingEvents(t *testing.T) {
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Config }, nil)
	s := statuspane.Snapshot{Model: "live", Provider: "provider", ConfigRows: []string{"OpenCode import enabled: true"}}
	p.Show(s)
	s.ConfigRows[0] = "mutated"
	before := text(p, 100, 25)
	for _, ev := range []xui.Event{
		xui.PasteEvent{Text: "secret pasted text"},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: '/'},
		xui.KeyEvent{Press: true, Code: xui.KeyEnter},
		xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 5, Y: 6},
	} {
		ctx := &components.EventContext{}
		require.True(t, p.HandleEvent(ctx, ev))
		assert.True(t, ctx.Consume)
		assert.Equal(t, before, text(p, 100, 25))
	}
	assert.Contains(t, before, "read-only")
	assert.Contains(t, before, "Effective session model: live")
	assert.Contains(t, before, "OpenCode import enabled: true")
	assert.NotContains(t, before, "mutated")
	assert.Empty(t, p.Draw(components.DrawContext{Max: components.Size{Width: 100, Height: 25}}).Children)
	press(p, xui.KeyEscape, 0)
	assert.False(t, p.Visible())
}
