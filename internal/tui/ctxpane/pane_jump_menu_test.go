package ctxpane

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

func paste(p *Pane, value string) {
	p.HandleEvent(&components.EventContext{}, xui.PasteEvent{Text: value})
}

func renderText(t *testing.T, p *Pane, width, height int) string {
	t.Helper()
	surface := p.Draw(components.DrawContext{
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

// The jump matches against kind and preview, so "/login" lands on the user
// row that says "fix the login bug" — the only row carrying that text.
func TestPaneJumpSelectsBestMatchLiveAndShrinksTheViewport(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	require.Equal(t, 5, p.cursor.Selected())

	require.True(t, press(t, p, xui.KeyRune, '/'))
	require.True(t, p.jump.Active())
	before := renderText(t, p, 80, 24)
	assert.Contains(t, before, "Jump", "the strip labels itself")
	assert.Contains(t, before, "Enter keep", "the footer speaks the jump scope")
	assert.Equal(t, 24-4-2, p.viewport, "the strip borrows two item rows")

	paste(p, "login")
	assert.Equal(t, 1, p.cursor.Selected(), "the best match is selected live")
	assert.Contains(t, renderText(t, p, 80, 24), "1 match")

	require.True(t, press(t, p, xui.KeyEnter, 0))
	assert.False(t, p.jump.Active())
	assert.Equal(t, 1, p.cursor.Selected(), "Enter keeps the jump's selection")
	assert.True(t, p.Visible(), "Enter closes the strip, not the browser")
}

func TestPaneJumpEscRestoresTheOriginSelection(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	require.True(t, press(t, p, xui.KeyUp, 0))
	require.Equal(t, 4, p.cursor.Selected())

	require.True(t, press(t, p, xui.KeyRune, '/'))
	paste(p, "login")
	require.Equal(t, 1, p.cursor.Selected())

	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.False(t, p.jump.Active())
	assert.Equal(t, 4, p.cursor.Selected(), "Esc walks back to the origin row")
	assert.True(t, p.Visible(), "Esc closes the strip, not the browser")
}

func TestPaneMenuListsCommandsAndRunsTrim(t *testing.T) {
	p, view, _, trimmed := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyRune, '.'))
	require.NotNil(t, p.menu)
	viewText := renderText(t, p, 80, 24)
	assert.Contains(t, viewText, "Actions", "the menu titles itself")
	assert.Contains(t, viewText, "View block (Enter)")
	assert.Contains(t, viewText, "Trim context up to here (t)")
	assert.Contains(t, viewText, "Delete block (Del)")
	assert.Contains(t, viewText, "Compact now (c)")
	assert.Contains(t, viewText, "Refresh (r)")
	assert.Contains(t, viewText, "Enter run", "the footer speaks the menu scope")

	require.True(t, press(t, p, xui.KeyDown, 0))
	require.True(t, press(t, p, xui.KeyEnter, 0))
	assert.Nil(t, p.menu, "running an item leaves the menu")
	require.True(t, p.confirm.Armed(), "the trim command still asks y/n")
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	assert.Equal(t, view.Items[5].EntryID, *trimmed, "the trim acts on the row the menu opened from")
}

func TestPaneMenuOnASummaryRowOffersNoTrimAndEscReturns(t *testing.T) {
	p, _, _, trimmed := newTestPane()
	p.Show()
	require.True(t, press(t, p, xui.KeyHome, 0))
	require.Equal(t, 0, p.cursor.Selected())

	require.True(t, press(t, p, xui.KeyRune, '.'))
	require.NotNil(t, p.menu)
	labels := make([]string, 0, len(p.menu))
	for _, item := range p.menu {
		labels = append(labels, item.Label)
	}
	assert.Equal(t, []string{"View block (Enter)", "Compact now (c)", "Refresh (r)"}, labels,
		"a summary row neither trims nor deletes")

	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.Nil(t, p.menu)
	assert.True(t, p.Visible(), "Esc closes the menu, not the browser")
	assert.Equal(t, 0, p.cursor.Selected(), "the list selection survives the menu round trip")
	assert.Empty(t, *trimmed)

	require.True(t, press(t, p, xui.KeyRune, '.'))
	require.True(t, press(t, p, xui.KeyEnter, 0))
	assert.True(t, p.popup, "the first menu item opens the block viewer")
	assert.NotContains(t, renderText(t, p, 60, 20), "Actions", "running an item leaves the menu")
}
