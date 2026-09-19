package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// MessageActions is what one transcript message offers through the action
// strip drawn at the right edge of a row it already owns: rewind the context
// to this message, fork a session from it, or ask a side question about the
// context at this point. The widget stays dumb, because the mapper closes
// every callback over the session entry the row stands for. A nil callback
// drops its button, which is how a row that is not a turn boundary ends up
// offering btw alone.
type MessageActions struct {
	OnRewind func()
	OnFork   func()
	OnAside  func()
	// Disabled says why the strip cannot act right now, in the words the
	// hint shows. While it is set the buttons are dimmed further and a
	// click does nothing; empty means the strip acts.
	Disabled string
}

// offers reports whether anything at all is wired. Nothing wired means no
// strip: a headless draw and a row with no session entry behind it both
// land here.
func (a MessageActions) offers() bool {
	return a.OnRewind != nil || a.OnFork != nil || a.OnAside != nil
}

// actionKind names one button of the strip.
type actionKind uint8

const (
	actionRewind actionKind = iota
	actionFork
	actionAside
)

// The buttons are words, with no glyph in front of them. A marker would have
// to be a symbol most terminal fonts draw as an empty box, and its width is
// the thing the terminals disagree about, while the whole geometry of the
// strip is measured in columns.
//
// Short labels are the same buttons on a row with no room for the words. The
// hint stays whole there, and on a one-letter button it is the only thing
// that says what a click would do.
const (
	rewindLabel = "rewind"
	forkLabel   = "fork"
	asideLabel  = "btw"
	rewindShort = "r"
	forkShort   = "f"
	asideShort  = "b"
)

// actionGap is the run of blanks between two buttons, actionPad the column
// the strip keeps clear of the right edge.
const (
	actionGap = 2
	actionPad = 1
)

// actionButton is where one button landed on the row.
type actionButton struct {
	kind  actionKind
	x, w  int
	label string
}

// actionStrip is where the last Draw put the strip. PointerShape,
// HoverTooltip and Handle all answer from it, so a cell acts only where the
// frame the user is looking at actually drew a button.
type actionStrip struct {
	row      int
	anchor   string
	disabled string
	buttons  []actionButton
}

// buttonAt names the button under a surface-local cell.
func (s actionStrip) buttonAt(x, y int) (actionButton, bool) {
	if len(s.buttons) == 0 || y != s.row {
		return actionButton{}, false
	}
	for _, b := range s.buttons {
		if x >= b.x && x < b.x+b.w {
			return b, true
		}
	}
	return actionButton{}, false
}

// messageActionBar is the action strip a transcript message carries: what it
// offers, and where the last frame put it. UserBlock and AssistantBlock
// embed it, so the prompt and the reply answer the pointer through one
// implementation instead of two that drift apart.
type messageActionBar struct {
	actions MessageActions
	strip   actionStrip
	painted paintedSpan
	armed   armedButton
}

// armedButton is the button a left press went down on, waiting for the
// release that would act on it.
type armedButton struct {
	kind  actionKind
	armed bool
}

// paintedSpan is the run of cells the strip wrote on the frame before. Every
// paint records one, and only a widget that hands out the same surface twice
// ever reads it back through erasePainted. The prompt records a span nobody
// asks about, which is cheaper than teaching paint whose surface it is
// drawing on, and keeps the two blocks running the same code.
type paintedSpan struct {
	row, x0, x1 int
	painted     bool
}

// erasePainted blanks the cells the strip wrote last frame and takes their
// chrome mark off. A widget whose surface survives between frames calls it
// before the next render, while the surface still holds the old frame. The
// reply's cache repaints only the rows whose content changed, and the strip
// is part of no row's content: without this, a strip that moves down as the
// body grows leaves a copy of itself on the row it came from, and the chrome
// mark it left keeps selection copy skipping that row's right edge forever.
// A widget that builds a fresh surface on every Draw needs none of it.
//
// The blank is the empty cell, which is what the reply's own repaint writes
// when it clears a row: the backdrop there is the terminal's own.
func (bar *messageActionBar) erasePainted(s *components.Surface) {
	p := bar.painted
	bar.painted = paintedSpan{}
	if !p.painted || s == nil || s.Buffer == nil || p.row < 0 || p.row >= s.Size.Height {
		return
	}
	for x := max(p.x0, 0); x < min(p.x1, s.Size.Width); x++ {
		s.Buffer[p.row*s.Size.Width+x] = xui.EmptyCell()
	}
	components.ClearChrome(s, p.x0, p.row, p.x1)
}

// SetActions wires what the row's strip offers. The mapper is the only
// caller: it alone knows which session entry the row stands for.
func (bar *messageActionBar) SetActions(a MessageActions) {
	if bar != nil {
		bar.actions = a
	}
}

// HoverRegion gives each button a region of its own, so crossing from one to
// the next repaints and the tint follows the pointer.
func (bar *messageActionBar) HoverRegion(x, y int) int {
	if b, ok := bar.strip.buttonAt(x, y); ok {
		return int(b.kind) + 1
	}
	return 0
}

// HoverTooltip says what a click on the button under the cell would do, or
// why it would do nothing.
func (bar *messageActionBar) HoverTooltip(x, y int) (string, bool) {
	b, ok := bar.strip.buttonAt(x, y)
	if !ok {
		return "", false
	}
	if bar.strip.disabled != "" {
		return bar.strip.disabled, true
	}
	switch b.kind {
	case actionRewind:
		return "rewind the context to " + bar.strip.anchor +
			"; whatever came after it leaves the context, and /rewind back brings it home", true
	case actionFork:
		return "fork the session from " + bar.strip.anchor + " into a new tab", true
	case actionAside:
		return "ask about the context as it stood at " + bar.strip.anchor +
			"; the answer stays out of the context", true
	}
	return "", false
}

// actionShape offers the hand over a button, the disabled ones included: the
// hand is what carries hover state to the widget, and a control that refuses
// to act still has to say why.
func (bar *messageActionBar) actionShape(x, y int) (string, bool) {
	if _, ok := bar.strip.buttonAt(x, y); ok {
		return components.ShapePointer, true
	}
	return "", false
}

// handleActionMouse works a button the way a button is worked anywhere: the
// press arms it, and the release acts, but only inside the button the press
// armed. A release that arrives without one, or over a neighbor, does
// nothing and is not swallowed, so a selection that began on the text and
// happened to end over the strip still finishes as a selection.
//
// The press itself is swallowed, the disabled ones included: a press left to
// the transcript would start a text selection under a control the user meant
// to click.
func (bar *messageActionBar) handleActionMouse(ctx *components.EventContext, ev xui.Event) {
	e, ok := ev.(xui.MouseEvent)
	if !ok || e.Button != xui.MouseLeft {
		return
	}
	b, over := bar.strip.buttonAt(e.X, e.Y)
	armed := bar.armed
	if !over || b.kind != armed.kind {
		bar.armed = armedButton{}
	}
	switch {
	case e.Action == xui.MousePress && over:
		if bar.strip.disabled == "" {
			bar.armed = armedButton{kind: b.kind, armed: true}
		}
		ctx.Consume = true
	case e.Action == xui.MouseRelease && over && armed.armed && armed.kind == b.kind:
		bar.armed = armedButton{}
		// The refusal is read again here, not only when the press armed the
		// button. A turn can start between the two halves of a click, from a
		// queued prompt or a follow-up, and the release must not act on a
		// strip that has meanwhile gone quiet.
		if bar.strip.disabled != "" {
			ctx.Consume = true
			return
		}
		bar.run(b.kind)
		ctx.ConsumeAndRedraw()
	}
}

// run calls the callback the button carries.
func (bar *messageActionBar) run(kind actionKind) {
	var fn func()
	switch kind {
	case actionRewind:
		fn = bar.actions.OnRewind
	case actionFork:
		fn = bar.actions.OnFork
	case actionAside:
		fn = bar.actions.OnAside
	}
	if fn != nil {
		fn()
	}
}

// disarmOffButton drops the arming when the frame shows the pointer is no
// longer on the armed button. A press that wandered off before the release
// must not act, and once the pointer leaves the block altogether no mouse
// event reaches it to say so; the hover state of the frame does.
func (bar *messageActionBar) disarmOffButton(ctx components.DrawContext, w components.Widget) {
	if !bar.armed.armed {
		return
	}
	if !components.Hovering(ctx, w) {
		bar.armed = armedButton{}
		return
	}
	if b, ok := bar.strip.buttonAt(ctx.Hover.X, ctx.Hover.Y); !ok || b.kind != bar.armed.kind {
		bar.armed = armedButton{}
	}
}

// layout places the offered buttons flush right on row, keeping actionGap
// between them, actionPad clear of the right edge and a blank column after
// the row's own content, which ends at contentEnd.
//
// The words are the strip to read, and the initials are the same buttons on
// a row with no room for words. An end-of-turn footer spelling out a long
// model name, its context size and the round's duration eats most of the row
// by itself, and that row is exactly where the closing reply's buttons
// belong, so a tight row shortens the strip instead of dropping it. Buttons
// vanish only when even the initials do not fit, and then the hint has
// nowhere to appear either.
func (bar *messageActionBar) layout(row, width, contentEnd int, anchor string, method xui.WidthMethod) {
	bar.strip = actionStrip{row: row, anchor: anchor, disabled: bar.actions.Disabled}
	if !bar.actions.offers() || row < 0 {
		return
	}
	for _, compact := range []bool{false, true} {
		if bar.place(bar.offeredButtons(method, compact), width, contentEnd) {
			return
		}
	}
}

// place fits the measured buttons into the room right of contentEnd and
// reports whether they went in.
func (bar *messageActionBar) place(buttons []actionButton, width, contentEnd int) bool {
	total := 0
	for i, b := range buttons {
		if i > 0 {
			total += actionGap
		}
		total += b.w
	}
	x := width - actionPad - total
	if total == 0 || x <= contentEnd {
		return false
	}
	for i := range buttons {
		buttons[i].x = x
		x += buttons[i].w + actionGap
	}
	bar.strip.buttons = buttons
	return true
}

// offeredButtons measures the labels of the wired actions, in strip order,
// as words or as initials. The width is what the draw method makes of the
// text, not its byte count: the strip is laid out in terminal columns.
func (bar *messageActionBar) offeredButtons(method xui.WidthMethod, compact bool) []actionButton {
	candidates := []struct {
		kind  actionKind
		label string
		short string
		fn    func()
	}{
		{actionRewind, rewindLabel, rewindShort, bar.actions.OnRewind},
		{actionFork, forkLabel, forkShort, bar.actions.OnFork},
		{actionAside, asideLabel, asideShort, bar.actions.OnAside},
	}
	buttons := make([]actionButton, 0, len(candidates))
	for _, c := range candidates {
		if c.fn == nil {
			continue
		}
		label := c.label
		if compact {
			label = c.short
		}
		buttons = append(buttons, actionButton{
			kind:  c.kind,
			w:     xui.StringWidth(label, method),
			label: label,
		})
	}
	return buttons
}

// paint draws the strip the last layout call placed, and under the pointer
// the hover tint of the button it rests on. Every cell of the strip's span is
// written, the blanks between buttons included, because AssistantBlock hands
// out a cached surface: a tint painted into it by an earlier frame would
// otherwise stay lit long after the pointer has left. Writing the same cells
// again costs nothing on an idle frame, since the renderer diffs and
// identical cells produce no output.
func (bar *messageActionBar) paint(
	s *components.Surface,
	ctx components.DrawContext,
	w components.Widget,
	th components.Theme,
	bg xui.Style,
) {
	bar.disarmOffButton(ctx, w)
	if len(bar.strip.buttons) == 0 {
		return
	}
	st := th.Muted
	st.Bg = bg.Bg
	if bar.strip.disabled != "" {
		st.Dim = true
	}
	var text strings.Builder
	for i, b := range bar.strip.buttons {
		if i > 0 {
			text.WriteString(strings.Repeat(" ", actionGap))
		}
		text.WriteString(b.label)
	}
	first := bar.strip.buttons[0]
	last := bar.strip.buttons[len(bar.strip.buttons)-1]
	s.Print(first.x, bar.strip.row, text.String(), st, ctx.Method)
	components.MarkChrome(s, first.x, bar.strip.row, last.x+last.w)
	bar.painted = paintedSpan{row: bar.strip.row, x0: first.x, x1: last.x + last.w, painted: true}
	if !components.Hovering(ctx, w) {
		return
	}
	if b, ok := bar.strip.buttonAt(ctx.Hover.X, ctx.Hover.Y); ok {
		components.ApplyHoverRect(s, b.x, b.x+b.w, bar.strip.row, bar.strip.row+1, th.BackgroundElement)
	}
}
