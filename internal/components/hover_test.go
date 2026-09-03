package components

import (
	"testing"

	"github.com/pulseaiclub/xui"
)

type hoverStub struct{ drew Surface }

func (*hoverStub) Handle(*EventContext, xui.Event) {}
func (h *hoverStub) Draw(_ DrawContext) Surface {
	h.drew = NewSurface(4, 1, h)
	return h.drew
}

// WithConstraints must carry Hover into child contexts: a widget deep in the
// tree decides its own affordance from it.
func TestWithConstraintsPropagatesHover(t *testing.T) {
	stub := &hoverStub{}
	h := &HoverState{Widget: stub, X: 1, Y: 0}
	parent := DrawContext{Hover: h}
	child := parent.WithConstraints(Size{}, Size{Width: 4, Height: 1})
	if child.Hover != h {
		t.Fatalf("child ctx lost Hover: %#v", child.Hover)
	}
	if !Hovering(child, stub) {
		t.Fatal("Hovering must report true for the named widget")
	}
	if Hovering(DrawContext{}, stub) {
		t.Fatal("Hovering must report false without a hover state")
	}
	other := &hoverStub{}
	if Hovering(child, other) {
		t.Fatal("Hovering must report false for a different widget")
	}
}

// ApplyHoverRows paints the affordance: rows in range get the background,
// rows outside stay untouched, and empty cells become painted spaces so the
// row reads as one bar.
func TestApplyHoverRowsPaintsBackground(t *testing.T) {
	s := NewSurface(3, 2, nil)
	s.SetCell(0, 0, xui.Cell{Char: "a", Width: 1})
	bg := xui.Style{Bg: xui.RGBColor(0x11, 0x22, 0x33)}
	ApplyHoverRows(&s, 0, 1, bg)
	if c := s.Buffer[0]; c.Style.Bg != xui.RGBColor(0x11, 0x22, 0x33) || c.Default {
		t.Fatalf("row 0 cell 0: %#v", c)
	}
	if c := s.Buffer[2]; c.Char != " " || c.Style.Bg != xui.RGBColor(0x11, 0x22, 0x33) {
		t.Fatalf("row 0 empty cell not painted: %#v", c)
	}
	if c := s.Buffer[3]; c.Style.Bg == bg.Bg || !c.Default {
		t.Fatalf("row 1 painted out of range: %#v", c)
	}
}

// HoverTitleRows is the gate every interactive block goes through: it paints
// only when the pointer is on this widget and the widget says a click would
// act. Both refusals leave the surface untouched.
func TestHoverTitleRowsPaintsOnlyForTheHoveredInteractiveWidget(t *testing.T) {
	bg := xui.Style{Bg: xui.RGBColor(0x11, 0x22, 0x33)}
	stub := &hoverStub{}
	other := &hoverStub{}
	ctx := DrawContext{Hover: &HoverState{Widget: stub}}

	painted := func(s Surface) bool { return s.Buffer[0].Style.Bg == bg.Bg }

	hovered := NewSurface(3, 2, stub)
	HoverTitleRows(ctx, &hovered, stub, 1, bg, true)
	if !painted(hovered) {
		t.Fatal("the hovered interactive widget must light up")
	}

	passive := NewSurface(3, 2, stub)
	HoverTitleRows(ctx, &passive, stub, 1, bg, false)
	if painted(passive) {
		t.Fatal("a widget with nothing to click must not light up")
	}

	elsewhere := NewSurface(3, 2, other)
	HoverTitleRows(ctx, &elsewhere, other, 1, bg, true)
	if painted(elsewhere) {
		t.Fatal("only the widget under the pointer lights up")
	}

	unhovered := NewSurface(3, 2, stub)
	HoverTitleRows(DrawContext{}, &unhovered, stub, 1, bg, true)
	if painted(unhovered) {
		t.Fatal("no hover state means no affordance")
	}
}
