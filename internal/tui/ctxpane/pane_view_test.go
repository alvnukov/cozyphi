package ctxpane

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// bodyFixtureView mirrors fixtureView but fills Body, the full block text the
// detail popup renders.
func bodyFixtureView() agent.ContextView {
	items := make([]session.ContextItem, 0, 6)
	running := 0
	add := func(kind, preview string, tokens int, lines int) {
		running += tokens
		body := preview
		if lines > 1 {
			body = preview + "\n" + strings.Repeat(fmt.Sprintf("line %d of %s\n", 2, kind), lines-1)
			body = strings.TrimRight(body, "\n")
		}
		items = append(items, session.ContextItem{
			EntryID:          kind + preview,
			Kind:             kind,
			Preview:          preview,
			Body:             body,
			Tokens:           tokens,
			CumulativeTokens: running,
		})
	}
	add("summary", "old conversation summarized", 500, 1)
	add("user", "fix the login bug", 120, 1)
	add("assistant", "looking at auth.go", 240, 30) // tall block for scroll tests
	add("tool", "read internal/auth/handler.go", 300, 1)
	add("user", "any progress?", 60, 1)
	add("assistant", "found it: nil check missing", 180, 1)
	return agent.ContextView{
		ContextReport: session.ContextReport{
			Items:           items,
			EstimatedTokens: running,
			LastCompaction:  &session.Compaction{Summary: "old conversation summarized"},
		},
		ContextWindow: 128000,
	}
}

func newViewPane(view *agent.ContextView) (*Pane, *[][]string) {
	var deleted [][]string
	p := New(
		components.DefaultTheme(),
		func() agent.ContextView { return *view },
		nil,
		nil,
		func(ids []string) error { deleted = append(deleted, ids); return nil },
		nil,
	)
	return p, &deleted
}

func shiftPress(t *testing.T, p *Pane, code xui.KeyCode) bool {
	t.Helper()
	ctx := &components.EventContext{}
	return p.HandleEvent(ctx, xui.KeyEvent{Press: true, Code: code, Mods: xui.ModShift})
}

// TestPaneEnterOpensBodyPopup: Enter on a row opens a popup with the block's
// full text instead of closing the browser.
func TestPaneEnterOpensBodyPopup(t *testing.T) {
	view := bodyFixtureView()
	p, _ := newViewPane(&view)
	p.Show()
	p.selected = 1 // "fix the login bug"

	require.True(t, press(t, p, xui.KeyEnter, 0))
	require.True(t, p.popup)
	require.True(t, p.Visible(), "the browser stays open behind the popup")

	s := p.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})
	require.Equal(t, 20, s.Size.Height)
	assert.Contains(t, components.SurfaceText(s), "fix the login bug", "popup shows the full body")
}

// TestPanePopupScrollsAndCloses: j/k scroll the popup body; Escape closes the
// popup but leaves the browser open.
func TestPanePopupScrollsAndCloses(t *testing.T) {
	view := bodyFixtureView()
	p, _ := newViewPane(&view)
	p.Show()
	p.selected = 2 // 30-line block

	require.True(t, press(t, p, xui.KeyEnter, 0))
	p.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})

	require.True(t, press(t, p, xui.KeyRune, 'j'))
	require.Equal(t, 1, p.popupScroll)
	require.True(t, press(t, p, xui.KeyRune, 'j'))
	require.Equal(t, 2, p.popupScroll)
	require.True(t, press(t, p, xui.KeyRune, 'k'))
	require.Equal(t, 1, p.popupScroll)

	require.True(t, press(t, p, xui.KeyEscape, 0))
	assert.False(t, p.popup)
	assert.True(t, p.Visible(), "Escape in the popup closes only the popup")
}

// TestPanePopupConsumesKeys: while the popup is open, list shortcuts must not
// fire — keys belong to the popup until it closes.
func TestPanePopupConsumesKeys(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.selected = 3

	require.True(t, press(t, p, xui.KeyEnter, 0))
	require.True(t, press(t, p, xui.KeyRune, 't'))
	assert.False(t, p.confirm, "trim shortcut stays dead behind the popup")
	require.True(t, press(t, p, xui.KeyDelete, 0))
	assert.Empty(t, *deleted, "delete stays dead behind the popup")

	require.True(t, press(t, p, xui.KeyEnter, 0))
	assert.False(t, p.popup, "Enter closes the popup")
	assert.True(t, p.Visible())
}

// TestPaneShiftArrowsRangeThenDelete: Shift+Down extends a range from the
// anchor; Delete asks for confirmation and hands every selected deletable
// block to the seam in one call (summary rows are skipped like trim).
func TestPaneShiftArrowsRangeThenDelete(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()

	require.True(t, press(t, p, xui.KeyHome, 0))
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.Equal(t, 1, p.selected)
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.Equal(t, 2, p.selected)
	require.True(t, p.ranging, "selection extends while shift is held")

	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'y'))

	require.Len(t, *deleted, 1, "one delete call for the whole range")
	assert.ElementsMatch(t, []string{"userfix the login bug", "assistantlooking at auth.go"}, (*deleted)[0])
	assert.False(t, p.confirm, "confirmation resets after acting")
}

// TestPaneDeleteSingleBlock: Delete without a range removes just the selected
// block after confirmation.
func TestPaneDeleteSingleBlock(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.selected = 3 // tool row

	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'y'))

	require.Len(t, *deleted, 1)
	assert.Equal(t, []string{"toolread internal/auth/handler.go"}, (*deleted)[0])
}

// TestPaneDeleteRefusedOnSummaryRow: the summary row is the compaction that
// shapes the context; deleting it makes no sense, exactly like trimming onto it.
func TestPaneDeleteRefusedOnSummaryRow(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.selected = 0

	require.True(t, press(t, p, xui.KeyDelete, 0))
	assert.False(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	assert.Empty(t, *deleted)
}

// TestPaneBackspaceAndDAreDelete: Backspace and plain "d" open the same
// confirmation as Delete; "n" cancels without calling the seam.
func TestPaneBackspaceAndDAreDelete(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.selected = 2

	require.True(t, press(t, p, xui.KeyRune, 'd'))
	require.True(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'n'))
	assert.False(t, p.confirm)
	assert.Empty(t, *deleted)

	require.True(t, press(t, p, xui.KeyBackspace, 0))
	require.True(t, p.confirm)
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	require.Len(t, *deleted, 1)
}

// TestPanePlainMoveClearsRange: a plain navigation key collapses the range
// back to a single-row selection.
func TestPanePlainMoveClearsRange(t *testing.T) {
	view := bodyFixtureView()
	p, _ := newViewPane(&view)
	p.Show()

	require.True(t, press(t, p, xui.KeyHome, 0))
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.True(t, p.ranging)
	require.True(t, press(t, p, xui.KeyDown, 0))
	assert.False(t, p.ranging, "plain move ends the range")
}
