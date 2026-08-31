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
	add := func(kind, preview string, tokens, lines int) {
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
	p.cursor.Select(1) // "fix the login bug"

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
	p.cursor.Select(2) // 30-line block

	require.True(t, press(t, p, xui.KeyEnter, 0))
	p.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})

	require.True(t, press(t, p, xui.KeyRune, 'j'))
	require.Equal(t, 1, p.popupText.Offset())
	require.True(t, press(t, p, xui.KeyRune, 'j'))
	require.Equal(t, 2, p.popupText.Offset())
	require.True(t, press(t, p, xui.KeyRune, 'k'))
	require.Equal(t, 1, p.popupText.Offset())

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
	p.cursor.Select(3)

	require.True(t, press(t, p, xui.KeyEnter, 0))
	require.True(t, press(t, p, xui.KeyRune, 't'))
	assert.False(t, trimArmed(p), "trim shortcut stays dead behind the popup")
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
	require.Equal(t, 1, p.cursor.Selected())
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.Equal(t, 2, p.cursor.Selected())
	require.True(t, p.ranging, "selection extends while shift is held")

	// The range tracks the caret through the anchor in both directions.
	require.True(t, shiftPress(t, p, xui.KeyUp))
	require.Equal(t, 1, p.cursor.Selected())
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.Equal(t, 2, p.cursor.Selected())

	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, deleteArmed(p))
	require.True(t, press(t, p, xui.KeyRune, 'y'))

	require.Len(t, *deleted, 1, "one delete call for the whole range")
	assert.ElementsMatch(t, []string{"userfix the login bug", "assistantlooking at auth.go"}, (*deleted)[0])
	assert.False(t, deleteArmed(p), "confirmation resets after acting")
}

// TestPaneDeleteSingleBlock: Delete without a range removes just the selected
// block after confirmation.
func TestPaneDeleteSingleBlock(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.cursor.Select(3) // tool row

	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, deleteArmed(p))
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
	p.cursor.Select(0)

	require.True(t, press(t, p, xui.KeyDelete, 0))
	assert.False(t, deleteArmed(p))
	require.True(t, press(t, p, xui.KeyRune, 'y'))
	assert.Empty(t, *deleted)
}

// TestPaneDIsDeleteAndBackspaceIsNot: plain "d" opens the same confirmation
// as Delete, while Backspace — which reads as "go back" in a list — deletes
// nothing and says which keys do; the next key clears the notice.
func TestPaneDIsDeleteAndBackspaceIsNot(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	p.Show()
	p.cursor.Select(2)

	require.True(t, press(t, p, xui.KeyRune, 'd'))
	require.True(t, deleteArmed(p))
	require.True(t, press(t, p, xui.KeyRune, 'n'))
	assert.False(t, deleteArmed(p))
	assert.Empty(t, *deleted)

	require.True(t, press(t, p, xui.KeyBackspace, 0))
	assert.False(t, deleteArmed(p), "backspace must not arm a delete")
	assert.Equal(t, "backspace does nothing here — press Del or d to delete", p.notice)
	assert.Empty(t, *deleted)

	require.True(t, press(t, p, xui.KeyDelete, 0))
	assert.Empty(t, p.notice, "the next key clears the notice")
	require.True(t, deleteArmed(p))
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

// TestPaneVimJumpKeysCollapseRange: gg and G are plain moves — a stale range
// anchor plus a jump must not arm a delete over a huge unintended range.
func TestPaneVimJumpKeysCollapseRange(t *testing.T) {
	view := bodyFixtureView()
	p, _ := newViewPane(&view)
	p.Show()

	require.True(t, press(t, p, xui.KeyHome, 0))
	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.True(t, p.ranging)
	require.True(t, press(t, p, xui.KeyRune, 'G'))
	assert.False(t, p.ranging, "G ends the range")

	require.True(t, shiftPress(t, p, xui.KeyDown))
	require.True(t, p.ranging)
	require.True(t, press(t, p, xui.KeyRune, 'g'))
	require.True(t, press(t, p, xui.KeyRune, 'g'))
	assert.False(t, p.ranging, "gg ends the range")
}

// TestPaneConfirmationsAreExclusive: arming a delete disarms a pending trim
// and vice versa — a double 'y' must fire exactly one action.
func TestPaneConfirmationsAreExclusive(t *testing.T) {
	view := bodyFixtureView()
	p, deleted := newViewPane(&view)
	var trimmed string
	p.onTrim = func(entryID string) error { trimmed = entryID; return nil }
	p.Show()
	p.cursor.Select(3)

	require.True(t, press(t, p, xui.KeyRune, 't'))
	require.True(t, trimArmed(p))
	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, deleteArmed(p))
	assert.False(t, trimArmed(p), "arming a delete disarms the pending trim")

	require.True(t, press(t, p, xui.KeyRune, 'y'))
	require.Len(t, *deleted, 1)
	assert.Empty(t, trimmed, "the second 'y' must not fire the displaced trim")
	assert.False(t, deleteArmed(p))

	// And the other direction.
	require.True(t, press(t, p, xui.KeyDelete, 0))
	require.True(t, deleteArmed(p))
	require.True(t, press(t, p, xui.KeyRune, 't'))
	require.True(t, trimArmed(p))
	assert.False(t, deleteArmed(p), "arming a trim disarms the pending delete")
	require.True(t, press(t, p, xui.KeyRune, 'n'))
	assert.False(t, trimArmed(p))
}

// The block viewer speaks the same dialect as the list: counts, gg/G and
// half screens land through the shared parser.
func TestPanePopupSpeaksTheMotionDialect(t *testing.T) {
	view := bodyFixtureView()
	p, _ := newViewPane(&view)
	p.Show()
	p.cursor.Select(2) // 30-line block
	require.True(t, press(t, p, xui.KeyEnter, 0))
	p.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})

	require.True(t, press(t, p, xui.KeyRune, '3'))
	require.True(t, press(t, p, xui.KeyRune, 'j'))
	assert.Equal(t, 3, p.popupText.Offset(), "3j scrolls three lines")

	require.True(t, press(t, p, xui.KeyRune, 'G'))
	positive := p.popupText.Offset()
	assert.Positive(t, positive, "G lands on the last screen")

	require.True(t, press(t, p, xui.KeyRune, 'g'))
	require.True(t, press(t, p, xui.KeyRune, 'g'))
	assert.Zero(t, p.popupText.Offset(), "gg comes back to the top")
	assert.True(t, p.popup, "motions never close the popup")
}
