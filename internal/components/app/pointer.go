package app

import (
	"github.com/alvnukov/cozyphi/internal/components"
)

// pointerShapeSeq builds the OSC 22 sequence that sets the mouse pointer
// shape; an empty shape resets the pointer to the terminal default.
func pointerShapeSeq(shape string) string {
	return "\x1b]22;" + shape + "\x1b\\"
}

// pointerShapeOf asks a widget what the pointer should look like over one
// of its cells. Widgets without interior structure — and misses, where w is
// nil — keep the terminal default.
func pointerShapeOf(w components.Widget, lx, ly int) string {
	if shaper, ok := w.(components.PointerShaper); ok {
		return shaper.PointerShape(lx, ly)
	}
	return ""
}

// updateHover resolves the pointer against the last painted frame —
// exactly what the user sees under it — and drives both pointer
// affordances: OSC 22 whenever the shape under the pointer changes (kitty,
// ghostty, foot, xterm reshape the caret), and App.hover for the
// terminal-independent highlight everywhere else. A frame is requested
// when the hovered widget or the shape changes; motion that changes
// neither costs nothing. A nil vx (tests, standalone draws) skips the
// raw write.
//
// The hit is returned so a mouse event is resolved against the frame once:
// the caller that delivers the click reuses this widget and its local
// coordinates rather than hit-testing the same frame again.
func (a *App) updateHover(x, y int) (components.Widget, int, int) {
	w, lx, ly := a.lastSurf.HitTestAt(x, y)
	shape := pointerShapeOf(w, lx, ly)
	if shape != a.pointerShape {
		a.pointerShape = shape
		if a.vx != nil {
			_, _ = a.vx.WriteRaw([]byte(pointerShapeSeq(shape)))
		}
	}
	// The visual affordance belongs to interactive regions only: where a
	// click would act, i.e. where the widget offers the pointer hand.
	var hover *components.HoverState
	if shape == components.ShapePointer {
		hover = &components.HoverState{Widget: w, X: lx, Y: ly}
	}
	if !sameHover(a.hover, hover) {
		a.hover = hover
		a.redraw = true
	}
	return w, lx, ly
}

// sameHover reports whether two hover states name the same widget; nil
// matches nil. Position inside the widget is deliberately ignored: the
// highlight covers the widget's interactive rows as a whole, and shape
// changes (title row → body) already carry their own frame.
func sameHover(a, b *components.HoverState) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Widget == b.Widget
}
