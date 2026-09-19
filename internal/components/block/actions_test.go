package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
)

// wideCtx is a draw wide enough for the whole action strip to fit.
func wideCtx() components.DrawContext {
	return components.DrawContext{
		Max:    components.Size{Width: 80, Height: 20},
		Method: xui.WidthUnicode,
	}
}

// hoverAt is the same draw with the pointer resting on one cell of w.
func hoverAt(w components.Widget, x, y int) components.DrawContext {
	ctx := wideCtx()
	ctx.Hover = &components.HoverState{Widget: w, X: x, Y: y}
	return ctx
}

// recorder counts the clicks each button delivered.
type recorder struct{ rewind, fork, aside int }

func (r *recorder) actions() block.MessageActions {
	return block.MessageActions{
		OnRewind: func() { r.rewind++ },
		OnFork:   func() { r.fork++ },
		OnAside:  func() { r.aside++ },
	}
}

// buttonCell finds a cell the named button occupies, by looking for the row
// the label lands on and the column its text starts at.
func buttonCell(t *testing.T, s components.Surface, label string) (int, int) {
	t.Helper()
	for y, row := range strings.Split(components.SurfaceText(s), "\n") {
		if before, _, found := strings.Cut(row, label); found {
			return xui.StringWidth(before, xui.WidthUnicode), y
		}
	}
	t.Fatalf("button %q is not on the surface:\n%s", label, components.SurfaceText(s))
	return 0, 0
}

// The prompt gets the whole strip on the blank padding row it already had,
// and its height does not budge.
func TestPromptStripKeepsTheBlockHeight(t *testing.T) {
	bare := &block.UserBlock{Text: "one line", Theme: components.DefaultTheme()}
	plain := bare.Draw(wideCtx())

	var rec recorder
	u := &block.UserBlock{Text: "one line", Theme: components.DefaultTheme()}
	u.SetActions(rec.actions())
	s := u.Draw(wideCtx())

	if s.Size.Height != plain.Size.Height {
		t.Fatalf("height with the strip = %d, without it %d", s.Size.Height, plain.Size.Height)
	}
	text := components.SurfaceText(s)
	for _, want := range []string{"rewind", "fork", "btw"} {
		if !strings.Contains(text, want) {
			t.Fatalf("strip is missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(strings.Split(text, "\n")[0], "rewind") {
		t.Fatalf("the strip left the padding row:\n%s", text)
	}
	// The glyphs are frame, not prompt text: a selection across the block
	// must not drag them into the clipboard.
	x, y := buttonCell(t, s, "rewind")
	if !s.IsChrome(x, y) {
		t.Fatal("strip cells are not marked chrome")
	}
}

// drawAt renders the widget at one width.
func drawAt(w components.Widget, width int) components.Surface {
	return w.Draw(components.DrawContext{
		Max:    components.Size{Width: width, Height: 20},
		Method: xui.WidthUnicode,
	})
}

// A row too tight for the words keeps the buttons as bare glyphs, and they
// still act and still explain themselves. Only a pane too narrow for the
// glyphs loses them.
func TestTightRowShortensTheStripBeforeDroppingIt(t *testing.T) {
	var rec recorder
	u := &block.UserBlock{Text: "hello", Theme: components.DefaultTheme()}
	u.SetActions(rec.actions())

	tight := drawAt(u, 20)
	text := components.SurfaceText(tight)
	if strings.Contains(text, "rewind") || strings.Contains(text, "btw") {
		t.Fatalf("the tight row kept the spelled-out strip:\n%s", text)
	}
	x, y := buttonCell(t, tight, "?")
	if got := u.PointerShape(x, y); got != components.ShapePointer {
		t.Fatalf("shape over the shortened button = %q", got)
	}
	if hint, ok := u.HoverTooltip(x, y); !ok || !strings.Contains(hint, "the answer stays out") {
		t.Fatalf("shortened button hint = %q, %v", hint, ok)
	}
	ctx := &components.EventContext{}
	u.Handle(ctx, xui.MouseEvent{X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress})
	if rec.aside != 1 {
		t.Fatalf("the shortened button did not act: aside %d", rec.aside)
	}

	bare := drawAt(u, 9)
	if strings.Contains(components.SurfaceText(bare), "?") {
		t.Fatalf("a pane with no room drew a strip:\n%s", components.SurfaceText(bare))
	}
	if u.PointerShape(7, 0) != components.ShapeText {
		t.Fatal("a prompt without a strip still offers the hand")
	}
}

// The end-of-turn footer can fill its row: a long model name with its context
// size and the round's duration leaves nothing on a narrow pane. The buttons
// move up a row rather than disappear, which is what the closing reply is
// promised.
func TestLongFooterKeepsTheReplyButtons(t *testing.T) {
	var rec recorder
	a := &block.AssistantBlock{
		Text:      "the answer",
		MetaLabel: "anthropic/claude-opus-4-5-20260101[1.2m]",
		MetaTail:  "2m 13s",
		Theme:     components.DefaultTheme(),
	}
	a.SetActions(rec.actions())

	s := drawAt(a, 60)
	if got := stripRows(s); got != 1 {
		t.Fatalf("the closing reply lost its buttons on a long footer:\n%s", components.SurfaceText(s))
	}
	x, y := buttonCell(t, s, "btw")
	if y != s.Size.Height-2 {
		t.Fatalf("the strip is on row %d, want the line above the footer", y)
	}
	ctx := &components.EventContext{}
	a.Handle(ctx, xui.MouseEvent{X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress})
	if rec.aside != 1 {
		t.Fatalf("the moved button did not act: aside %d", rec.aside)
	}
	// The same reply on a wide pane keeps the strip on the footer itself.
	wide := drawAt(a, 100)
	if _, y := buttonCell(t, wide, "btw"); y != wide.Size.Height-1 {
		t.Fatalf("a wide pane put the strip on row %d, want the footer row", y)
	}
}

// The hand, the tint and the hint all follow the pointer to the one button
// it rests on, and stop at its edges.
func TestPromptStripLightsUpUnderThePointer(t *testing.T) {
	th := components.DefaultTheme()
	var rec recorder
	u := &block.UserBlock{Text: "hello", Theme: th}
	u.SetActions(rec.actions())
	s := u.Draw(wideCtx())
	fx, fy := buttonCell(t, s, "fork")

	if got := u.PointerShape(fx, fy); got != components.ShapePointer {
		t.Fatalf("shape over the fork button = %q", got)
	}
	if got := u.PointerShape(4, fy); got != components.ShapeText {
		t.Fatalf("shape left of the strip = %q", got)
	}
	if u.HoverRegion(fx, fy) == 0 || u.HoverRegion(fx, fy) == u.HoverRegion(fx+30, fy) {
		t.Fatal("the buttons do not carry separate hover regions")
	}
	hint, ok := u.HoverTooltip(fx, fy)
	if !ok || !strings.Contains(hint, "fork the session from this prompt") {
		t.Fatalf("fork hint = %q, %v", hint, ok)
	}
	if _, ok := u.HoverTooltip(4, fy); ok {
		t.Fatal("a cell outside the strip carries a hint")
	}

	s = u.Draw(hoverAt(u, fx, fy))
	if got := cellBg(s, fx, fy); got != th.BackgroundElement.Bg {
		t.Fatalf("hovered fork bg = %v, want %v", got, th.BackgroundElement.Bg)
	}
	rx, _ := buttonCell(t, s, "rewind")
	if got := cellBg(s, rx, fy); got == th.BackgroundElement.Bg {
		t.Fatal("the tint spilled onto the neighboring button")
	}
}

// cellBg reads one cell's background.
func cellBg(s components.Surface, x, y int) xui.Color {
	return s.Buffer[y*s.Size.Width+x].Style.Bg
}

// A press lands on the button under it and nowhere else, and it is consumed,
// so the row never starts a text selection under a control.
func TestPromptStripRunsTheButtonUnderTheClick(t *testing.T) {
	var rec recorder
	u := &block.UserBlock{Text: "hello", Theme: components.DefaultTheme()}
	u.SetActions(rec.actions())
	s := u.Draw(wideCtx())
	ax, ay := buttonCell(t, s, "btw")

	ctx := &components.EventContext{}
	u.Handle(ctx, xui.MouseEvent{X: ax, Y: ay, Button: xui.MouseLeft, Action: xui.MousePress})
	if rec.aside != 1 || rec.rewind != 0 || rec.fork != 0 {
		t.Fatalf("clicks = rewind %d, fork %d, aside %d", rec.rewind, rec.fork, rec.aside)
	}
	if !ctx.Consume {
		t.Fatal("the strip let the press through to text selection")
	}

	miss := &components.EventContext{}
	u.Handle(miss, xui.MouseEvent{X: 4, Y: ay, Button: xui.MouseLeft, Action: xui.MousePress})
	if rec.aside != 1 || miss.Consume {
		t.Fatalf("a press beside the strip acted: aside %d, consume %v", rec.aside, miss.Consume)
	}
}

// While a turn runs the strip explains itself and refuses, and the press is
// still swallowed so the click does not turn into a selection.
func TestDisabledStripRefusesAndSaysWhy(t *testing.T) {
	var rec recorder
	actions := rec.actions()
	actions.Disabled = "the turn is still running"
	u := &block.UserBlock{Text: "hello", Theme: components.DefaultTheme()}
	u.SetActions(actions)
	s := u.Draw(wideCtx())
	x, y := buttonCell(t, s, "rewind")

	if hint, ok := u.HoverTooltip(x, y); !ok || hint != "the turn is still running" {
		t.Fatalf("disabled hint = %q, %v", hint, ok)
	}
	ctx := &components.EventContext{}
	u.Handle(ctx, xui.MouseEvent{X: x, Y: y, Button: xui.MouseLeft, Action: xui.MousePress})
	if rec.rewind != 0 {
		t.Fatal("a disabled button acted")
	}
	if !ctx.Consume {
		t.Fatal("a disabled press fell through to text selection")
	}
}

// The reply carries the strip on its footer row, and a reply that is not a
// turn boundary carries the side question alone.
func TestReplyStripFollowsTheTurnBoundary(t *testing.T) {
	var rec recorder
	closing := &block.AssistantBlock{
		Text:      "the answer",
		MetaLabel: "model",
		MetaTail:  "4s",
		Theme:     components.DefaultTheme(),
	}
	closing.SetActions(rec.actions())
	s := closing.Draw(wideCtx())
	text := components.SurfaceText(s)
	for _, want := range []string{"rewind", "fork", "btw"} {
		if !strings.Contains(text, want) {
			t.Fatalf("closing reply is missing %q:\n%s", want, text)
		}
	}
	if _, y := buttonCell(t, s, "btw"); y != s.Size.Height-1 {
		t.Fatalf("the strip is on row %d, want the footer row %d", y, s.Size.Height-1)
	}

	mid := &block.AssistantBlock{Text: "thinking out loud", Theme: components.DefaultTheme()}
	mid.SetActions(block.MessageActions{OnAside: func() { rec.aside++ }})
	text = components.SurfaceText(mid.Draw(wideCtx()))
	if strings.Contains(text, "rewind") || strings.Contains(text, "fork") {
		t.Fatalf("a mid-turn reply offers a cut of the context:\n%s", text)
	}
	if !strings.Contains(text, "btw") {
		t.Fatalf("a mid-turn reply lost the side question:\n%s", text)
	}
}

// The reply hands out a cached surface. A frame drawn under the pointer must
// not leave its tint behind for every frame after it.
func TestReplyStripTintDoesNotStickInTheCache(t *testing.T) {
	th := components.DefaultTheme()
	var rec recorder
	a := &block.AssistantBlock{Text: "cached answer", MetaLabel: "model", Theme: th}
	a.SetActions(rec.actions())
	s := a.Draw(wideCtx())
	x, y := buttonCell(t, s, "rewind")

	s = a.Draw(hoverAt(a, x, y))
	if got := cellBg(s, x, y); got != th.BackgroundElement.Bg {
		t.Fatalf("hovered reply button bg = %v, want %v", got, th.BackgroundElement.Bg)
	}
	s = a.Draw(wideCtx())
	if got := cellBg(s, x, y); got == th.BackgroundElement.Bg {
		t.Fatal("the hover tint stayed lit in the cached surface")
	}
}

// A row nobody wired gets no strip at all: a failed turn leaves a row with no
// session entry behind it, and the mapper answers by wiring nothing.
func TestUnwiredMessageDrawsNoStrip(t *testing.T) {
	a := &block.AssistantBlock{Text: "run failed", MetaLabel: "model", Theme: components.DefaultTheme()}
	s := a.Draw(wideCtx())
	if strings.Contains(components.SurfaceText(s), "btw") {
		t.Fatalf("an unwired reply drew a strip:\n%s", components.SurfaceText(s))
	}
	if a.PointerShape(70, s.Size.Height-1) != components.ShapeText {
		t.Fatal("an unwired reply offers the hand")
	}
}

// stripRows reports how many rows of the surface carry a strip, counted by
// the button that every strip has.
func stripRows(s components.Surface) int {
	n := 0
	for row := range strings.SplitSeq(components.SurfaceText(s), "\n") {
		if strings.Contains(row, asideMark) {
			n++
		}
	}
	return n
}

// asideMark is the label of the button the side question always shows.
const asideMark = "btw"

// A reply grows while it streams, and the strip travels down with it. The
// cached surface keeps the rows that did not change, so the strip must leave
// no copy of itself on the row it came from.
func TestReplyStripMovesWithAGrowingBody(t *testing.T) {
	var rec recorder
	a := &block.AssistantBlock{Text: "first line", Theme: components.DefaultTheme()}
	a.SetActions(block.MessageActions{OnAside: func() { rec.aside++ }})

	a.Draw(wideCtx())
	a.Text = "first line\n\nsecond line\n\nthird line"
	s := a.Draw(wideCtx())
	if got := stripRows(s); got != 1 {
		t.Fatalf("the strip is on %d rows after the body grew:\n%s", got, components.SurfaceText(s))
	}
	a.Text = "first line"
	s = a.Draw(wideCtx())
	if got := stripRows(s); got != 1 {
		t.Fatalf("the strip is on %d rows after the body shrank:\n%s", got, components.SurfaceText(s))
	}
}

// Closing a round adds the footer row, and the strip moves onto it. The old
// copy must go, and so must the chrome marks it left behind.
func TestReplyStripMovesOntoTheFooterRow(t *testing.T) {
	var rec recorder
	a := &block.AssistantBlock{Text: "the answer", Theme: components.DefaultTheme()}
	a.SetActions(rec.actions())
	s := a.Draw(wideCtx())
	oldX, oldY := buttonCell(t, s, "btw")

	a.MetaLabel = "model"
	a.MetaTail = "4s"
	s = a.Draw(wideCtx())
	if got := stripRows(s); got != 1 {
		t.Fatalf("the strip is on %d rows after the round closed:\n%s", got, components.SurfaceText(s))
	}
	if _, y := buttonCell(t, s, "btw"); y != s.Size.Height-1 {
		t.Fatalf("the strip stayed on row %d, want the footer row %d", y, s.Size.Height-1)
	}
	if s.IsChrome(oldX, oldY) {
		t.Fatal("the row the strip left is still marked chrome, so copy skips its right edge")
	}
}

// Two draws that see the same thing produce the same cells. The renderer
// diffs frames, and a cell rewritten with a different value is a byte on an
// idle frame.
func TestRepeatedDrawsProduceTheSameCells(t *testing.T) {
	var rec recorder
	th := components.DefaultTheme()

	a := &block.AssistantBlock{Text: "an answer", MetaLabel: "model", Theme: th}
	a.SetActions(rec.actions())
	first := append([]xui.Cell(nil), a.Draw(wideCtx()).Buffer...)
	if !sameCells(first, a.Draw(wideCtx()).Buffer) {
		t.Fatal("an idle reply frame rewrites cells")
	}
	x, y := buttonCell(t, a.Draw(wideCtx()), "fork")
	hovered := append([]xui.Cell(nil), a.Draw(hoverAt(a, x, y)).Buffer...)
	if !sameCells(hovered, a.Draw(hoverAt(a, x, y)).Buffer) {
		t.Fatal("a hovered reply frame rewrites cells")
	}

	u := &block.UserBlock{Text: "a prompt", Theme: th}
	u.SetActions(rec.actions())
	before := append([]xui.Cell(nil), u.Draw(wideCtx()).Buffer...)
	if !sameCells(before, u.Draw(wideCtx()).Buffer) {
		t.Fatal("an idle prompt frame rewrites cells")
	}
}

// sameCells compares two painted buffers cell by cell.
func sameCells(a, b []xui.Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
