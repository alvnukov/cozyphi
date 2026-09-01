package overlays

import (
	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

// SetBottomOrigin records where the editor placed the bottom panel this
// frame, completing the rectangle mouse hits are resolved against.
func (o *Overlays) SetBottomOrigin(x, y int) {
	if o != nil {
		o.panelX, o.panelY = x, y
	}
}

// HandleAskMouse gives the modal ask the mouse the way it already owns the
// keyboard. A wheel notch moves the option selection, a click on an option
// row selects it, a click on the selected option activates it, and every
// other mouse event stops here: a click that fell through the modal used
// to resize the sidebar or scroll the transcript while the user thought
// they were answering the dialog.
func (o *Overlays) HandleAskMouse(ctx *components.EventContext, e xui.MouseEvent) bool {
	if o == nil || (o.perm == nil && o.cont == nil && o.question == nil) {
		return false
	}
	switch {
	case e.Button == xui.MouseWheelUp || e.Button == xui.MouseWheelDown:
		if !o.askTakingText() {
			o.wheelAsk(e)
			ctx.ConsumeAndRedraw()
			return true
		}
	case e.Action == xui.MousePress && e.Button == xui.MouseLeft && !o.askTakingText():
		if idx, ok := o.askOptionAt(e.X, e.Y); ok {
			o.clickAskOption(idx)
			ctx.ConsumeAndRedraw()
			return true
		}
	}
	// Everything else — clicks outside the panel or on prose rows, drags,
	// motion — dies here too: a modal is modal for every input channel.
	ctx.Consume = true
	return true
}

// wheelAsk steps the option ring one option per wheel notch; the short
// fixed lists a modal carries need no faster scroll.
func (o *Overlays) wheelAsk(e xui.MouseEvent) {
	step := max(e.Wheel, 1)
	if e.Button == xui.MouseWheelUp {
		step = -step
	}
	o.askRing().Step(step)
	o.clearAskHint()
}

// askOptionAt maps screen coordinates to the option drawn there, using the
// panel rectangle recorded on the last frame.
func (o *Overlays) askOptionAt(x, y int) (int, bool) {
	if o.panelW <= 0 || x < o.panelX || x >= o.panelX+o.panelW {
		return 0, false
	}
	row := y - o.panelY - 1 // body rows paint below the top border
	if row < 0 || row >= o.panelH-2 {
		return 0, false
	}
	innerW := askInnerWidth(o.panelW)
	switch {
	case o.perm != nil:
		return o.perm.optionAt(o.theme, innerW, o.panelH, o.panelMethod, row)
	case o.cont != nil:
		return o.cont.optionAt(o.theme, innerW, o.panelMethod, row)
	default:
		return o.question.optionAt(o.theme, innerW, o.panelMethod, row)
	}
}

// clickAskOption applies the click contract of every ask: the first click
// on an option selects it, a click on the already selected option
// activates it — the same two-beat gesture as moving and pressing Enter.
func (o *Overlays) clickAskOption(idx int) {
	o.clearAskHint()
	switch {
	case o.perm != nil:
		if idx == o.perm.ring.Selected() {
			o.acceptPermissionOption(askOption(idx))
			return
		}
		o.perm.ring.Select(idx)
	case o.cont != nil:
		if idx == o.cont.ring.Selected() {
			o.acceptContinueOption(idx)
			return
		}
		o.cont.ring.Select(idx)
	case o.question != nil:
		st := o.question
		st.ring.SetLen(st.optionCount())
		if idx == st.ring.Selected() {
			o.acceptQuestionOption(st)
			return
		}
		st.ring.Select(idx)
	}
}

// askRing is the option selection of whichever ask is showing.
func (o *Overlays) askRing() *browse.Ring {
	switch {
	case o.perm != nil:
		return &o.perm.ring
	case o.cont != nil:
		return &o.cont.ring
	default:
		return &o.question.ring
	}
}

// askTakingText reports whether the active ask is in a text-entry mode,
// where the pointer has no option to land on.
func (o *Overlays) askTakingText() bool {
	return (o.perm != nil && o.perm.feedbackMode) ||
		(o.question != nil && o.question.editing)
}

// clearAskHint drops the unbound-key hint: the mouse just did something.
func (o *Overlays) clearAskHint() {
	switch {
	case o.perm != nil:
		o.perm.hint = ""
	case o.cont != nil:
		o.cont.hint = ""
	case o.question != nil:
		o.question.hint = ""
	}
}

// optionAt maps a panel body row to the permission option drawn there,
// recomputing exactly the rows the panel painted — fit included.
func (st *permAskState) optionAt(
	th components.Theme,
	innerW, height int,
	method xui.WidthMethod,
	row int,
) (int, bool) {
	body, answer := st.askRows(th, innerW, method)
	body = fitAskBody(th, body, height-2, answer, innerW, method)
	return optionForRow(row, len(body), answer, st.optionBlocks(th, askPrimary(th), innerW, method))
}

func (st *continueAskState) optionAt(
	th components.Theme,
	innerW int,
	method xui.WidthMethod,
	row int,
) (int, bool) {
	body, answer := st.askRows(th, innerW, method)
	return optionForRow(row, len(body), answer, st.optionBlocks(th, askPrimary(th), innerW, method))
}

func (st *questionAskState) optionAt(
	th components.Theme,
	innerW int,
	method xui.WidthMethod,
	row int,
) (int, bool) {
	body, answer := st.askRows(th, innerW, method)
	return optionForRow(row, len(body), answer, questionOptionBlocks(st, th, askPrimary(th), innerW, method))
}

// optionForRow finds the option block covering a painted body row. The
// last answer rows of the body are the option blocks and then the hint,
// in order; everything before them is prose no click can select.
func optionForRow(row, bodyLen, answer int, blocks [][]components.RichLine) (int, bool) {
	idx := row - (bodyLen - answer)
	if idx < 0 || row >= bodyLen {
		return 0, false
	}
	for i, block := range blocks {
		idx -= len(block)
		if idx < 0 {
			return i, true
		}
	}
	return 0, false // the hint row
}
