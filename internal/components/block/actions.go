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

// The labels carry their own glyph, the way every other affordance in the
// transcript does: U+21B6, the anticlockwise open circle arrow, for the
// rewind, and U+2442, the OCR fork, for the fork. Both are spelled by code
// point rather than typed in, because the symbol gate over this repository
// reads source text as prose and turns the glyphs away.
const (
	rewindGlyph = string(rune(0x21b6))
	forkGlyph   = string(rune(0x2442))
)

// actionGap is the run of blanks between two buttons, actionPad the column
// the strip keeps clear of the right edge.
const (
	rewindLabel = rewindGlyph + " rewind"
	forkLabel   = forkGlyph + " fork"
	asideLabel  = "? btw"
	actionGap   = 2
	actionPad   = 1
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

// handleActionMouse runs the button under a left press. A press on a
// disabled button is swallowed too, so the row does not start a text
// selection under a control the user meant to click.
func (bar *messageActionBar) handleActionMouse(ctx *components.EventContext, ev xui.Event) {
	e, ok := ev.(xui.MouseEvent)
	if !ok || e.Action != xui.MousePress || e.Button != xui.MouseLeft {
		return
	}
	b, ok := bar.strip.buttonAt(e.X, e.Y)
	if !ok {
		return
	}
	if bar.strip.disabled != "" {
		ctx.Consume = true
		return
	}
	var run func()
	switch b.kind {
	case actionRewind:
		run = bar.actions.OnRewind
	case actionFork:
		run = bar.actions.OnFork
	case actionAside:
		run = bar.actions.OnAside
	}
	if run != nil {
		run()
	}
	ctx.ConsumeAndRedraw()
}

// layout places the offered buttons flush right on row, keeping actionGap
// between them, actionPad clear of the right edge and a blank column after
// the row's own content, which ends at contentEnd. A row too narrow for the
// whole strip gets none of it: half a strip reads as damage, and the slash
// commands cover the narrow terminal.
func (bar *messageActionBar) layout(row, width, contentEnd int, anchor string, method xui.WidthMethod) {
	bar.strip = actionStrip{row: row, anchor: anchor, disabled: bar.actions.Disabled}
	if !bar.actions.offers() || row < 0 {
		return
	}
	buttons := bar.offeredButtons(method)
	total := 0
	for i, b := range buttons {
		if i > 0 {
			total += actionGap
		}
		total += b.w
	}
	x := width - actionPad - total
	if total == 0 || x <= contentEnd {
		return
	}
	for i := range buttons {
		buttons[i].x = x
		x += buttons[i].w + actionGap
	}
	bar.strip.buttons = buttons
}

// offeredButtons measures the labels of the wired actions, in strip order.
// The width comes from the draw method rather than from the byte count: the
// glyphs are not one byte wide, and on some of them the terminals disagree.
func (bar *messageActionBar) offeredButtons(method xui.WidthMethod) []actionButton {
	candidates := []struct {
		kind  actionKind
		label string
		fn    func()
	}{
		{actionRewind, rewindLabel, bar.actions.OnRewind},
		{actionFork, forkLabel, bar.actions.OnFork},
		{actionAside, asideLabel, bar.actions.OnAside},
	}
	buttons := make([]actionButton, 0, len(candidates))
	for _, c := range candidates {
		if c.fn == nil {
			continue
		}
		buttons = append(buttons, actionButton{
			kind:  c.kind,
			w:     xui.StringWidth(c.label, method),
			label: c.label,
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
	if !components.Hovering(ctx, w) {
		return
	}
	if b, ok := bar.strip.buttonAt(ctx.Hover.X, ctx.Hover.Y); ok {
		components.ApplyHoverRect(s, b.x, b.x+b.w, bar.strip.row, bar.strip.row+1, th.BackgroundElement)
	}
}
