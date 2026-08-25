package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func drawEditor(c *ChatInput, w int) components.Surface {
	return c.Draw(components.DrawContext{
		Max:    components.Size{Width: w, Height: 20},
		Method: xui.WidthUnicode,
	})
}

// Visual click→caret: a mouse press inside the editor body must move the
// caret to the clicked cell. Text starts at column 3 on the editor's first
// body row (row 1: frame pad).
func TestChatInputMouseClickMovesCaret(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 11}
	drawEditor(c, 60)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 3 + 6, Y: 1})
	if c.Cursor != 6 {
		t.Fatalf("click on col 6: cursor = %d, want 6", c.Cursor)
	}
	if !ctx.Consume {
		t.Fatal("click inside editor must be consumed")
	}
}

// Drag selection: press, drag, release selects the covered text and leaves
// the caret at the selection end.
func TestChatInputMouseDragSelects(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 11}
	drawEditor(c, 60)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 3, Y: 1})
	c.Handle(ctx, xui.MouseEvent{Action: xui.MouseDrag, Button: xui.MouseLeft, X: 3 + 5, Y: 1})
	c.Handle(ctx, xui.MouseEvent{Action: xui.MouseRelease, Button: xui.MouseLeft, X: 3 + 5, Y: 1})
	if got := c.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText = %q, want %q", got, "hello")
	}
	if c.Cursor != 5 {
		t.Fatalf("cursor after select = %d, want 5 (selection end)", c.Cursor)
	}
}

// Shift+Right extends a selection from the caret anchor.
func TestChatInputShiftArrowsSelect(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello", Cursor: 0}
	ctx := &components.EventContext{}
	for range 5 {
		c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRight, Mods: xui.ModShift, Press: true})
	}
	if got := c.SelectedText(); got != "hello" {
		t.Fatalf("SelectedText = %q, want %q", got, "hello")
	}
	if c.Cursor != 5 {
		t.Fatalf("cursor = %d, want 5", c.Cursor)
	}
	// A plain move collapses the selection again.
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyLeft, Press: true})
	if c.HasSelection() {
		t.Fatal("plain Left must collapse the selection")
	}
}

// Selection replacement: typing prints over the selection and consumes it.
func TestChatInputTypingReplacesSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 5}
	c.SetSelection(0, 5)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'H', Press: true})
	if c.Value != "H world" {
		t.Fatalf("value = %q, want %q", c.Value, "H world")
	}
	if c.HasSelection() {
		t.Fatal("selection must be consumed by typing")
	}
}

// Backspace and Delete over a selection remove it whole.
func TestChatInputDeleteKeyRemovesSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 5}
	c.SetSelection(0, 5)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyDelete, Press: true})
	if c.Value != " world" || c.Cursor != 0 {
		t.Fatalf("value = %q cursor = %d", c.Value, c.Cursor)
	}
}

// Ctrl+C over an active selection copies the composer text through OnCopy
// and consumes the press, so the App runtime does not quit.
func TestChatInputCtrlCCopiesSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world"}
	c.SetSelection(0, 5)
	copied := ""
	c.OnCopy = func(s string) bool { copied = s; return true }
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl, Press: true})
	if copied != "hello" {
		t.Fatalf("copied = %q, want %q", copied, "hello")
	}
	if !ctx.Consume {
		t.Fatal("copy chord must be consumed so App does not quit")
	}
	if !c.AcceptCopyKey(xui.KeyEvent{Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl, Press: true}) {
		t.Fatal("AcceptCopyKey must claim Ctrl+C with a selection")
	}
	c.ClearSelection()
	if c.AcceptCopyKey(xui.KeyEvent{Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl, Press: true}) {
		t.Fatal("AcceptCopyKey must not claim Ctrl+C without a selection")
	}
}

// Ctrl+X cuts: copies through OnCopy, then deletes the selection.
func TestChatInputCtrlXCuts(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 5}
	c.SetSelection(0, 5)
	copied := ""
	c.OnCopy = func(s string) bool { copied = s; return true }
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'x', Mods: xui.ModCtrl, Press: true})
	if copied != "hello" {
		t.Fatalf("copied = %q", copied)
	}
	if c.Value != " world" {
		t.Fatalf("value = %q, want %q", c.Value, " world")
	}
	if !ctx.Consume {
		t.Fatal("cut must be consumed")
	}
}

// Ctrl+A selects the whole value.
func TestChatInputCtrlASelectsAll(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 3}
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'a', Mods: xui.ModCtrl, Press: true})
	if !c.HasSelection() || c.SelectedText() != "hello world" {
		t.Fatalf("select-all: hasSel=%v text=%q", c.HasSelection(), c.SelectedText())
	}
}

// Paste over a selection replaces it.
func TestChatInputPasteReplacesSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 5}
	c.SetSelection(0, 5)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.PasteEvent{Text: "goodbye"})
	if c.Value != "goodbye world" {
		t.Fatalf("value = %q", c.Value)
	}
}

// Word wrap: a line with spaces must break on the space, not mid-word.
func TestChatInputWrapBreaksOnWords(t *testing.T) {
	c := &ChatInput{MinBodyRows: 1, MaxBodyRows: 8, Value: "hello world", Cursor: 11}
	s := drawEditor(c, 15) // innerW = 10: "hello " then "world"
	row0, row1 := rowString(s, 1), rowString(s, 2)
	if !strings.Contains(row0, "hello") || strings.Contains(row0, "world") {
		t.Fatalf("row0 = %q, want the whole first word", row0)
	}
	if strings.Contains(row0[:6], "w") {
		t.Fatalf("row0 = %q: word must not be split across rows", row0)
	}
	if !strings.Contains(row1, "world") {
		t.Fatalf("row1 = %q, want the second word on the next row", row1)
	}
}

// Up/Down move across soft-wrapped rows keeping the column; Home/End address
// the visual row. Up from the end of "cccc" keeps column 4 — on the row
// above that column sits right after "aaaa".
func TestChatInputVisualRowNavigation(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 8, Value: "aaaa bbbb cccc", Cursor: 14}
	drawEditor(c, 15) // rows: "aaaa bbbb " | "cccc"
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyUp, Press: true})
	if c.Cursor != len("aaaa") {
		t.Fatalf("Up from end: cursor = %d, want %d (column kept on row 0)", c.Cursor, len("aaaa"))
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyHome, Press: true})
	if c.Cursor != 0 {
		t.Fatalf("Home: cursor = %d, want 0", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnd, Press: true})
	if c.Cursor != len("aaaa bbbb ") {
		t.Fatalf("End: cursor = %d, want end of visual row 0", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyDown, Press: true})
	if c.Cursor != len("aaaa bbbb ") {
		t.Fatalf("Down on the last visual row must stay put, got %d", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnd, Press: true})
	if c.Cursor != len("aaaa bbbb cccc") {
		t.Fatalf("End on last row: cursor = %d, want value end", c.Cursor)
	}
}

// Ctrl+Left/Right jump word boundaries; Ctrl+Backspace deletes a word.
func TestChatInputWordNavigationAndDelete(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "one two three", Cursor: 13}
	drawEditor(c, 60)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyLeft, Mods: xui.ModCtrl, Press: true})
	if c.Cursor != len("one two ") {
		t.Fatalf("Ctrl+Left: cursor = %d", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyLeft, Mods: xui.ModCtrl, Press: true})
	if c.Cursor != len("one ") {
		t.Fatalf("second Ctrl+Left: cursor = %d", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRight, Mods: xui.ModCtrl, Press: true})
	if c.Cursor != len("one two") {
		t.Fatalf("Ctrl+Right: cursor = %d, want end of the current word", c.Cursor)
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyBackspace, Mods: xui.ModCtrl, Press: true})
	if c.Value != "one three" {
		t.Fatalf("Ctrl+Backspace: value = %q", c.Value)
	}
}

// CJK click: a press on the continuation column of a wide glyph snaps the
// caret to the glyph start, never between its halves.
func TestChatInputClickSnapsToGlyphStart(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "中文明", Cursor: 9}
	drawEditor(c, 60)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 3 + 1, Y: 1})
	if c.Cursor != 0 {
		t.Fatalf("click continuation col: cursor = %d, want 0 (glyph start)", c.Cursor)
	}
}

// Selection paints: rows inside the selection carry the selection background.
func TestChatInputSelectionPainted(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Theme: components.DefaultTheme()}
	c.SetSelection(0, 5)
	s := drawEditor(c, 60)
	selBg := components.DefaultTheme().SelectionBg.Bg
	hit := false
	for x := 3; x < 3+5; x++ {
		if s.Buffer[60+x].Style.Bg.Equal(selBg) {
			hit = true
		}
	}
	if !hit {
		t.Fatal("selected cells must carry the selection background")
	}
}

// Ctrl+Shift+C must copy the selection: xui's CtrlC() does not exclude
// Shift, so the App quit path would swallow it unless the composer claims
// every Ctrl+c copy chord.
func TestChatInputCtrlShiftCCopiesSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world"}
	c.SetSelection(0, 5)
	copied := ""
	c.OnCopy = func(s string) bool { copied = s; return true }
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl | xui.ModShift, Press: true})
	if copied != "hello" {
		t.Fatalf("Ctrl+Shift+C copied = %q, want %q", copied, "hello")
	}
	if !c.AcceptCopyKey(xui.KeyEvent{Code: xui.KeyRune, Rune: 'c', Mods: xui.ModCtrl | xui.ModShift, Press: true}) {
		t.Fatal("AcceptCopyKey must claim Ctrl+Shift+C with a selection")
	}
}

// All-motion tracking (?1003) delivers buttonless hovers: while a drag is
// stale (its release landed on another widget), hover must not move the
// caret or arm a selection.
func TestChatInputHoverDoesNotMutateCaret(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello world", Cursor: 11}
	drawEditor(c, 60)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 3, Y: 1})
	c.Handle(ctx, xui.MouseEvent{Action: xui.MouseDrag, Button: xui.MouseLeft, X: 3 + 5, Y: 1})
	if c.Cursor != 5 {
		t.Fatalf("drag: cursor = %d, want 5", c.Cursor)
	}
	// Stale drag + plain hover (no button): nothing may change.
	c.Handle(ctx, xui.MouseEvent{Action: xui.MouseMotion, X: 3 + 8, Y: 1})
	if c.Cursor != 5 {
		t.Fatalf("hover during stale drag moved cursor to %d", c.Cursor)
	}
	if got := c.SelectedText(); got != "hello" {
		t.Fatalf("hover changed selection to %q", got)
	}
}

// Shift+Right at the value end must not arm a phantom collapsed selection
// that would swallow the next Backspace.
func TestChatInputShiftAtEndNoPhantomSelection(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "hello", Cursor: 5}
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRight, Mods: xui.ModShift, Press: true})
	if c.HasSelection() {
		t.Fatal("Shift+Right at end must not report a selection")
	}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})
	if c.Value != "hell" || c.Cursor != 4 {
		t.Fatalf("backspace after phantom: value=%q cursor=%d", c.Value, c.Cursor)
	}
}

// End on a row that soft-wrapped at a space (width < inner width) keeps the
// caret on that row, not on the start of the next one.
func TestChatInputEndOnShortWrappedRowStaysOnRow(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 8, Value: "ab 中文字", Cursor: 0}
	drawEditor(c, 9) // innerW=4: rows "ab " / "中文" / "字"
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnd, Press: true})
	if c.Cursor != 3 {
		t.Fatalf("End: cursor = %d, want 3 (end of short row 0)", c.Cursor)
	}
	s := drawEditor(c, 9)
	if s.Cursor == nil || s.Cursor.Y != 1 {
		t.Fatalf("caret row = %+v, want y=1 (same visual row)", s.Cursor)
	}
}
