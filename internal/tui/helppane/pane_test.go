package helppane

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func newTestPane() (*Pane, *int) {
	closes := 0
	return New(components.DefaultTheme(), func() { closes++ }), &closes
}

func press(t *testing.T, p *Pane, code xui.KeyCode, r rune) bool {
	t.Helper()
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
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

func TestPaneShowHideNotifiesOnce(t *testing.T) {
	p, closes := newTestPane()
	assert.False(t, p.Visible())

	p.Show()
	require.True(t, p.Visible())
	assert.Zero(t, *closes)

	p.Hide()
	assert.False(t, p.Visible())
	assert.Equal(t, 1, *closes)

	p.Hide()
	assert.Equal(t, 1, *closes, "hiding a hidden pane must not fire onClose again")
}

func TestPaneClosingKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		code xui.KeyCode
		r    rune
	}{
		{"escape", xui.KeyEscape, 0},
		{"f1", xui.KeyF1, 0},
		{"q", xui.KeyRune, 'q'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, closes := newTestPane()
			p.Show()
			require.True(t, press(t, p, tc.code, tc.r))
			assert.False(t, p.Visible())
			assert.Equal(t, 1, *closes)
		})
	}
}

func TestPaneConsumesTypingWhileVisible(t *testing.T) {
	p, _ := newTestPane()
	p.Show()
	assert.True(t, press(t, p, xui.KeyRune, 'x'), "keys never reach the composer")

	p.Hide()
	assert.False(t, press(t, p, xui.KeyRune, 'x'), "hidden pane consumes nothing")
}

func TestPaneScrollClampsToRows(t *testing.T) {
	p, _ := newTestPane()
	p.Show()
	// The viewport is only known after a draw; scrolling before one still
	// has to stay inside the rows.
	renderText(t, p, 80, 24)
	bottom := len(p.rows) - p.viewport
	require.Positive(t, bottom, "the catalog must overflow a 24-row screen")

	require.True(t, press(t, p, xui.KeyUp, 0))
	assert.Zero(t, p.view.Offset(), "scrolling up from the top stays at the top")

	require.True(t, press(t, p, xui.KeyRune, 'G'))
	assert.Equal(t, bottom, p.view.Offset(), "the last screen sits flush with the bottom")

	require.True(t, press(t, p, xui.KeyDown, 0))
	assert.Equal(t, bottom, p.view.Offset(), "scrolling past the end stays at the end")

	require.True(t, press(t, p, xui.KeyRune, 'g'))
	assert.Equal(t, bottom, p.view.Offset(), "a single g only opens a gg")
	require.True(t, press(t, p, xui.KeyRune, 'g'))
	assert.Zero(t, p.view.Offset())

	require.True(t, press(t, p, xui.KeyPageDown, 0))
	assert.Equal(t, p.viewport-1, p.view.Offset(), "a page keeps one row of overlap")
}

// TestPaneSpeaksTheMotionDialect spot-checks that the pane wires the shared
// parser in: counts, half screens and jumps all land. The dialect itself is
// pinned in the browse package.
func TestPaneSpeaksTheMotionDialect(t *testing.T) {
	p, _ := newTestPane()
	p.Show()
	renderText(t, p, 80, 24)

	require.True(t, press(t, p, xui.KeyRune, '3'))
	require.True(t, press(t, p, xui.KeyRune, 'j'))
	assert.Equal(t, 3, p.view.Offset(), "3j scrolls three rows")

	require.True(t, p.HandleEvent(&components.EventContext{},
		xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'd', Mods: xui.ModCtrl}))
	assert.Equal(t, 3+p.viewport/2, p.view.Offset(), "Ctrl+D scrolls half a screen")

	require.True(t, press(t, p, xui.KeyRune, '5'))
	require.True(t, press(t, p, xui.KeyRune, 'G'))
	assert.Equal(t, 4, p.view.Offset(), "5G puts row five first")
}

func TestPaneWheelScrolls(t *testing.T) {
	p, _ := newTestPane()
	p.Show()
	renderText(t, p, 80, 24)

	ctx := &components.EventContext{}
	require.True(t, p.HandleEvent(ctx, xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: 2}))
	assert.Equal(t, 2*browse.WheelStep, p.view.Offset(), "a notch is three rows")
	require.True(t, p.HandleEvent(ctx, xui.MouseEvent{Button: xui.MouseWheelUp, Wheel: 2}))
	assert.Zero(t, p.view.Offset())
}

func TestPaneDrawsCatalogAndFooter(t *testing.T) {
	p, _ := newTestPane()
	p.Show()
	out := renderText(t, p, 100, 40)

	assert.Contains(t, out, "Keyboard shortcuts")
	assert.Contains(t, out, keys.Hints(keys.ScopeHelp), "the screen documents its own keys")

	global, ok := keys.Find(keys.ScopeGlobal)
	require.True(t, ok)
	assert.Contains(t, out, global.Title)
	assert.Contains(t, out, global.Bindings[0].Label())
	assert.Contains(t, out, global.Bindings[0].Desc)
}

// TestPaneAlignsKeyColumnByDisplayWidth guards the two-column layout against
// byte-length padding: "↑↓" is three bytes wide and one column narrower than
// its own length, so a byte-padded column would leave descriptions ragged.
func TestPaneAlignsKeyColumnByDisplayWidth(t *testing.T) {
	p := &Pane{
		theme: components.DefaultTheme(),
		rows: []row{
			{kind: rowBinding, key: "↑↓", text: "arrows"},
			{kind: rowBinding, key: "Ctrl+G", text: "letters"},
		},
		visible: true,
	}
	out := renderText(t, p, 40, 6)
	lines := strings.Split(out, "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	// One cell renders one char, so the column is the rune offset — not the
	// byte offset, which is exactly what this test is here to catch.
	column := func(line, want string) int {
		i := strings.Index(line, want)
		require.GreaterOrEqual(t, i, 0, "%q must appear in %q", want, line)
		return len([]rune(line[:i]))
	}
	assert.Equal(t, column(lines[1], "arrows"), column(lines[2], "letters"),
		"descriptions start in the same column")
}
