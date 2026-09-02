package components

import (
	"github.com/pulseaiclub/xui"
)

// HoverState names the widget the pointer rests on, with coordinates local
// to that widget's surface. The app resolves it against the last painted
// frame and publishes it into the next DrawContext; each widget decides
// from it whether its interactive region lights up.
type HoverState struct {
	Widget Widget
	X, Y   int
}

// Hovering reports whether ctx.Hover names w: the affordance belongs to the
// widget actually under the pointer, never to look-alikes.
func Hovering(ctx DrawContext, w Widget) bool {
	return ctx.Hover != nil && ctx.Hover.Widget == w
}

// ApplyHoverRows paints the terminal-independent hover affordance: a quiet
// element background under rows [y0, y1) of s. OSC 22 reshapes the pointer
// only in terminals that implement it; this tint is what every other one
// shows. Glyphs and foregrounds stay; empty cells become painted spaces so
// the region reads as one bar.
func ApplyHoverRows(s *Surface, y0, y1 int, bg xui.Style) {
	fillRowRangeBg(s, 0, y0, y1, bg)
}

// HoverTitleRows is the whole hover affordance of an interactive block: when
// the pointer rests on w and interactive says a click would do something,
// rows [0, h) of s take the tint. Every expandable widget lights up the same
// way, and this is the one place that decides what the same way means — a
// new block gets the affordance by calling this, not by remembering to paint
// three lines of its own.
func HoverTitleRows(ctx DrawContext, s *Surface, w Widget, h int, bg xui.Style, interactive bool) {
	if !interactive || !Hovering(ctx, w) {
		return
	}
	ApplyHoverRows(s, 0, h, bg)
}
