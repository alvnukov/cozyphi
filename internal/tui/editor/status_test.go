package editor

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestStatusEmbedsConfigAndOwnsInput(t *testing.T) {
	e := newEditorWithSettings(t)
	e.composer.Chat.Value = "untouched draft"
	var closed []string
	e.ConfigureStatusTabs(func() string { return statuspane.Config }, func(tab string) { closed = append(closed, tab) })
	e.ShowStatus()
	require.True(t, e.modalActive())
	require.Equal(t, statuspane.Config, e.status.Tab())
	root := e.Draw(components.DrawContext{Max: components.Size{Width: 100, Height: 35}, Method: xui.WidthUnicode})
	assert.True(t, surfaceContains(root, "[config]"))
	assert.True(t, surfaceContains(root, "Harness settings"))

	// Config keeps its own Tab navigation and fuzzy search while F2/F3 belong
	// to the dashboard. Nothing is routed back to the composer.
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyTab})
	assert.Equal(t, settings.TabGeneral, e.settings.State().Tab)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: '/'})
	assert.True(t, e.settings.State().Jumping)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.True(t, e.status.Visible(), "escape closes search before the dashboard")
	assert.False(t, e.settings.State().Jumping)

	// The draft survives dashboard tab changes; Save still runs the existing
	// store and closes the embedded editor/dashboard together.
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyF3})
	assert.Equal(t, statuspane.Usage, e.status.Tab())
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyF2})
	assert.Equal(t, settings.TabGeneral, e.settings.State().Tab)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl})
	assert.False(t, e.status.Visible())
	assert.False(t, e.settings.Visible())
	assert.Equal(t, []string{statuspane.Config}, closed)
	assert.Equal(t, "untouched draft", e.composer.Chat.Value)
}

func TestStatusMetadataIsAllowlisted(t *testing.T) {
	e := newEditorWithSettings(t)
	e.ConfigureStatusTabs(func() string { return statuspane.Status }, nil)
	e.ShowStatus()
	root := e.Draw(components.DrawContext{Max: components.Size{Width: 140, Height: 45}, Method: xui.WidthUnicode})
	assert.True(t, surfaceContains(root, "Current session:"))
	assert.True(t, surfaceContains(root, "Global config:"))
	assert.True(t, surfaceContains(root, "Account: unavailable"))
	assert.False(t, surfaceContains(root, "test-key"))
	assert.False(t, surfaceContains(root, "127.0.0.1:9"))
}

func surfaceContains(s components.Surface, value string) bool {
	if strings.Contains(components.SurfaceText(s), value) {
		return true
	}
	for _, child := range s.Children {
		if surfaceContains(child.Surface, value) {
			return true
		}
	}
	return false
}
