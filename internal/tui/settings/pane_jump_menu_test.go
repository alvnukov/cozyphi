package settings_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

func paste(p *settings.Pane, value string) {
	p.HandleEvent(&components.EventContext{}, xui.PasteEvent{Text: value})
}

func renderText(t *testing.T, pane *settings.Pane, width, height int) string {
	t.Helper()
	surface := pane.Draw(components.DrawContext{
		Max: components.Size{Width: width, Height: height}, Method: xui.WidthUnicode,
	})
	screen := xui.NewScreen(width, height)
	window := xui.NewWindow(screen)
	window.Clear()
	surface.Render(window)
	var text strings.Builder
	for y := range height {
		for x := range width {
			text.WriteString(screen.GetCell(x, y).Char)
		}
		text.WriteByte('\n')
	}
	return text.String()
}

// The General tab is a fixed eight-row list, so the jump's ranking is
// deterministic: "scope" tightens onto the "Scope: global" row alone.
func TestPaneJumpSelectsBestMatchLiveAndRendersStrip(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	require.Equal(t, settings.TabGeneral, pane.State().Tab)

	require.True(t, key(pane, xui.KeyRune, '/', 0))
	require.True(t, pane.State().Jumping)
	view := renderText(t, pane, 64, 14)
	assert.Contains(t, view, "Jump", "the strip labels itself")
	assert.Contains(t, view, "Enter keep", "the footer speaks the jump scope")

	paste(pane, "scope")
	assert.Equal(t, 5, pane.State().Selected, "the best match is selected live")
	assert.Contains(t, renderText(t, pane, 64, 14), "1 match")

	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Jumping)
	assert.Equal(t, 5, pane.State().Selected, "Enter keeps the jump's selection")
}

func TestPaneJumpEscRestoresTheOriginSelection(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.Equal(t, 1, pane.State().Selected)

	require.True(t, key(pane, xui.KeyRune, '/', 0))
	paste(pane, "scope")
	require.Equal(t, 5, pane.State().Selected)

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.State().Jumping)
	assert.Equal(t, 1, pane.State().Selected, "Esc walks back to the origin row")
	assert.True(t, pane.Visible(), "Esc closes the strip, not the modal")
}

func TestPaneMenuListsActionsAndRunsApply(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	require.True(t, key(pane, xui.KeyRune, '.', 0))
	view := renderText(t, pane, 72, 16)
	assert.Contains(t, view, "Actions", "the menu titles itself")
	assert.Contains(t, view, "Next tab (Tab)")
	assert.Contains(t, view, "Apply changes (Ctrl+S)")
	assert.Contains(t, view, "Enter run", "the footer speaks the menu scope")

	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.Len(t, store.applied, 1, "the menu item runs exactly as Ctrl+S would")
	assert.False(t, pane.Visible(), "a successful apply closes the modal")
}

func TestPaneMenuRunsNextTabAndEscReturnsToTheList(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	require.True(t, key(pane, xui.KeyRune, '.', 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Equal(t, settings.TabGeneral, pane.State().Tab)
	assert.NotContains(t, renderText(t, pane, 64, 14), "Actions", "running an item leaves the menu")

	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.Equal(t, 1, pane.State().Selected)
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.True(t, pane.Visible(), "Esc closes the menu, not the modal")
	assert.Equal(t, 1, pane.State().Selected, "the list selection survives the menu round trip")
	assert.Empty(t, store.applied)
}
