package editor

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestStatusReadOnlyConfigOwnsInput(t *testing.T) {
	e := newEditorWithSettings(t)
	e.composer.Chat.Value = "untouched draft"
	var closed []string
	e.ConfigureStatusTabs(func() string { return statuspane.Config }, func(tab string) { closed = append(closed, tab) })
	e.ShowStatus()
	require.True(t, e.modalActive())
	require.Equal(t, statuspane.Config, e.status.Tab())
	root := e.Draw(components.DrawContext{Max: components.Size{Width: 100, Height: 35}, Method: xui.WidthUnicode})
	assert.True(t, surfaceContains(root, "[Config]"))
	assert.True(t, surfaceContains(root, "Harness settings"))

	before := e.settings.State()
	for _, ev := range []xui.Event{
		xui.PasteEvent{Text: "should not change settings or composer"},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: '/'},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl},
		xui.KeyEvent{Press: true, Code: xui.KeyEnter},
	} {
		e.Handle(&components.EventContext{}, ev)
		assert.True(t, e.status.Visible())
		assert.False(t, e.settings.Visible())
		assert.Equal(t, before, e.settings.State())
	}
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyTab})
	assert.Equal(t, statuspane.Usage, e.status.Tab())
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyF2})
	assert.Equal(t, statuspane.Config, e.status.Tab())
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
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
