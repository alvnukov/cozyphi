package ctxpane

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

func fixtureView() agent.ContextView {
	items := make([]session.ContextItem, 0, 6)
	running := 0
	add := func(kind, preview string, tokens int) {
		running += tokens
		items = append(items, session.ContextItem{
			EntryID:          kind + preview,
			Kind:             kind,
			Preview:          preview,
			Tokens:           tokens,
			CumulativeTokens: running,
		})
	}
	add("summary", "old conversation summarized", 500)
	add("user", "fix the login bug", 120)
	add("assistant", "looking at auth.go", 240)
	add("tool", "read internal/auth/handler.go", 300)
	add("user", "any progress?", 60)
	add("assistant", "found it: nil check missing", 180)
	return agent.ContextView{
		ContextReport: session.ContextReport{
			Items:           items,
			EstimatedTokens: running,
			LastCompaction:  &session.Compaction{Summary: "old conversation summarized"},
		},
		ContextWindow:   128000,
		ContextTokens:   12000,
		TokenSource:     "provider",
		ThresholdTokens: 111616,
	}
}

func newTestPane() (*Pane, *agent.ContextView, *int, *string) {
	calls := 0
	view := fixtureView()
	var trimmed string
	p := New(
		components.DefaultTheme(),
		func() agent.ContextView { return view },
		func() { calls++ },
		func(entryID string) error { trimmed = entryID; return nil },
		nil,
		nil,
	)
	return p, &view, &calls, &trimmed
}

func press(t *testing.T, p *Pane, code xui.KeyCode, rune rune) bool {
	t.Helper()
	ctx := &components.EventContext{}
	return p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: code, Rune: rune})
}

func TestPaneShowSelectsNewestAndDraws(t *testing.T) {
	p, view, _, _ := newTestPane()
	p.Show()

	require.True(t, p.Visible())
	require.Equal(t, len(view.Items)-1, p.selected, "Show lands on the newest entry")

	s := p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}})
	require.Equal(t, 24, s.Size.Height)
	require.Positive(t, p.viewport)
}

func TestPaneNavigationAndClamp(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyUp, 0))
	assert.Equal(t, 4, p.selected)
	require.True(t, press(t, p, xui.KeyHome, 0))
	assert.Equal(t, 0, p.selected)
	require.True(t, press(t, p, xui.KeyUp, 0), "selection clamps at the top")
	assert.Equal(t, 0, p.selected)
	require.True(t, press(t, p, xui.KeyEnd, 0))
	assert.Equal(t, 5, p.selected)
}

func TestPaneConsumesTypingWhileVisible(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	assert.True(t, press(t, p, xui.KeyRune, 'x'), "keys never reach the composer")

	p.Hide()
	assert.False(t, press(t, p, xui.KeyRune, 'x'), "hidden pane consumes nothing")
}

func TestPaneTrimFlowConfirmsThenActs(t *testing.T) {
	p, _, _, trimmed := newTestPane()
	p.Show()
	p.selected = 1 // "fix the login bug"

	require.True(t, press(t, p, xui.KeyRune, 't'))
	require.True(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'n'))
	assert.False(t, p.confirm)

	require.True(t, press(t, p, xui.KeyRune, 't'))
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	assert.Equal(t, "userfix the login bug", *trimmed)
	assert.False(t, p.confirm, "confirmation resets after acting")
}

func TestPaneTrimRefusedOnSummaryRow(t *testing.T) {
	p, _, _, trimmed := newTestPane()
	p.Show()
	p.selected = 0 // summary row

	require.True(t, press(t, p, xui.KeyRune, 't'))
	assert.False(t, p.confirm, "trimming onto a summary is a no-op")
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	assert.Empty(t, *trimmed)
}

func TestPaneCompactClosesAndFires(t *testing.T) {
	p, _, calls, _ := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyRune, 'c'))
	assert.Equal(t, 1, *calls)
	assert.False(t, p.Visible(), "compaction events own the screen afterwards")
}

func TestPaneEscapeCloses(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.False(t, p.Visible())
}

func TestPaneHeaderNumbers(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	h := p.header()
	assert.Contains(t, h, "12k")
	assert.Contains(t, h, "provider")
	assert.Contains(t, h, "128k")
	assert.Contains(t, p.compactionLine(), "old conversation summarized")
}

func drawSmall(t *testing.T, p *Pane) {
	t.Helper()
	p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 8}})
	require.Equal(t, 4, p.viewport, "8 rows minus 4 chrome rows")
}

func wheel(t *testing.T, p *Pane, button xui.MouseButton) bool {
	t.Helper()
	return p.HandleEvent(&components.EventContext{}, xui.MouseEvent{Button: button, Wheel: 1})
}

// Wheel scrolls the viewport without dragging it back to the selected row:
// after Home the selection sits at the top, and scrolling must still move.
func TestPaneWheelScrollsFreeOfSelection(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	drawSmall(t, p)
	require.True(t, press(t, p, xui.KeyHome, 0))

	require.True(t, wheel(t, p, xui.MouseWheelDown))
	assert.Equal(t, 1, p.scroll, "wheel moves the window even with selection pinned to top")
}

// xui delivers Shift+G as Rune 'G' with ModShift; it must jump to the end
// instead of being swallowed by the modifier guard.
func TestPaneShiftGJumpsToEnd(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	require.True(t, press(t, p, xui.KeyHome, 0))

	ctx := &components.EventContext{}
	require.True(t, p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'G', Mods: xui.ModShift}))
	assert.Equal(t, 5, p.selected)
}

func TestPaneVimHalfPage(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	drawSmall(t, p) // viewport 4 → half page 2

	require.True(t, press(t, p, xui.KeyHome, 0))
	require.True(t, press(t, p, xui.KeyRune, 'd'))
	assert.Equal(t, 0, p.selected, "plain d never moves the selection; only Ctrl+d is half-page")
	ctx := &components.EventContext{}
	require.True(t, p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'd', Mods: xui.ModCtrl}))
	assert.Equal(t, 2, p.selected, "Ctrl+d moves half a page down")
	require.True(t, p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'u', Mods: xui.ModCtrl}))
	assert.Equal(t, 0, p.selected, "Ctrl+u moves half a page up")
}

func TestPaneVimCountPrefix(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyRune, '3'))
	require.True(t, press(t, p, xui.KeyRune, 'k'))
	assert.Equal(t, 2, p.selected, "3k moves three rows up")
}

func TestPaneVimDoubleG(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()

	require.True(t, press(t, p, xui.KeyRune, 'g'))
	require.True(t, press(t, p, xui.KeyRune, 'k'))
	assert.Equal(t, 4, p.selected, "a lone g waits for a second key, it is not Home")

	require.True(t, press(t, p, xui.KeyRune, 'g'))
	require.True(t, press(t, p, xui.KeyRune, 'g'))
	assert.Equal(t, 0, p.selected, "gg jumps to the top")
}

func TestPanePageKeysMoveSelection(t *testing.T) {
	p, _, _, _ := newTestPane()
	p.Show()
	drawSmall(t, p) // viewport 4 → page 3

	require.True(t, press(t, p, xui.KeyPageUp, 0))
	assert.Equal(t, 2, p.selected)
	require.True(t, press(t, p, xui.KeyPageDown, 0))
	assert.Equal(t, 5, p.selected)
}

// Closing the pane (Escape) must notify the shell exactly once so it can
// hand the keyboard back to the composer.
func TestPaneCloseNotifiesOnce(t *testing.T) {
	closed := 0
	view := fixtureView()
	p := New(
		components.DefaultTheme(),
		func() agent.ContextView { return view },
		nil, nil, nil,
		func() { closed++ },
	)
	p.Show()
	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.Equal(t, 1, closed)
	p.Hide() // already hidden — no second notification
	assert.Equal(t, 1, closed)
}
