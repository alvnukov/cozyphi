package app

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// shapeStub is a widget whose whole surface reports one pointer shape.
type shapeStub struct {
	shape string
}

func (*shapeStub) Handle(*components.EventContext, xui.Event) {}

func (s *shapeStub) PointerShape(_, _ int) string { return s.shape }

func (s *shapeStub) Draw(components.DrawContext) components.Surface {
	return components.Surface{Size: components.Size{Width: 10, Height: 2}, Widget: s}
}

// plainStub reports no shape at all.
type plainStub struct{}

func (*plainStub) Handle(*components.EventContext, xui.Event) {}

func (p *plainStub) Draw(components.DrawContext) components.Surface {
	return components.Surface{Size: components.Size{Width: 10, Height: 10}, Widget: p}
}

func TestPointerShapeSeq(t *testing.T) {
	if got := pointerShapeSeq(components.ShapePointer); got != "\x1b]22;pointer\x1b\\" {
		t.Fatalf("set seq = %q", got)
	}
	if got := pointerShapeSeq(""); got != "\x1b]22;\x1b\\" {
		t.Fatalf("reset seq = %q", got)
	}
}

// The shape comes from the deepest widget under the cell, resolved through
// the same pass the app runs on a real mouse event.
func TestUpdateHoverTakesTheShapeFromTheDeepestWidget(t *testing.T) {
	root := (&plainStub{}).Draw(components.DrawContext{})
	root.Children = []components.SubSurface{
		{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: (&shapeStub{shape: components.ShapePointer}).Draw(components.DrawContext{}),
		},
		{
			Origin:  components.Point{X: 0, Y: 2},
			Surface: (&shapeStub{shape: components.ShapeText}).Draw(components.DrawContext{}),
		},
	}
	a := &App{lastSurf: root}

	cases := []struct {
		name string
		x, y int
		want string
	}{
		{"title row", 5, 1, components.ShapePointer},
		{"body row", 5, 3, components.ShapeText},
		{"plain root", 5, 8, ""},
		{"miss", 50, 50, ""},
	}
	for _, tc := range cases {
		a.updateHover(tc.x, tc.y)
		if a.pointerShape != tc.want {
			t.Fatalf("%s shape = %q, want %q", tc.name, a.pointerShape, tc.want)
		}
	}
}

// The hit updateHover returns is the one the click is delivered to: a mouse
// event resolves the frame once.
func TestUpdateHoverReturnsTheHitItResolved(t *testing.T) {
	stub := &shapeStub{shape: components.ShapePointer}
	root := (&plainStub{}).Draw(components.DrawContext{})
	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 2}, Surface: stub.Draw(components.DrawContext{})},
	}
	a := &App{lastSurf: root}

	w, lx, ly := a.updateHover(5, 3)

	if w != components.Widget(stub) {
		t.Fatalf("hit = %#v, want the stub under the pointer", w)
	}
	if lx != 5 || ly != 1 {
		t.Fatalf("local coords = (%d,%d), want (5,1)", lx, ly)
	}
}

// updateHover resolves the pointer against the last painted frame: the
// interactive target becomes App.hover and a frame is requested when it (or
// the pointer shape) changes; OSC 22 keeps flowing on shape changes. Motion
// that changes neither costs no frame.
func TestUpdateHoverTracksInteractiveTarget(t *testing.T) {
	stub := &shapeStub{shape: components.ShapePointer}
	root := (&plainStub{}).Draw(components.DrawContext{})
	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: stub.Draw(components.DrawContext{})},
	}
	a := &App{lastSurf: root}

	// Move onto the pointer-shaped stub: hover set, frame requested.
	a.updateHover(5, 1)
	if a.hover == nil || a.hover.Widget != components.Widget(stub) || !a.redraw {
		t.Fatalf("hover = %#v redraw = %v", a.hover, a.redraw)
	}
	if a.pointerShape != components.ShapePointer {
		t.Fatalf("shape = %q", a.pointerShape)
	}

	// Motion within the same widget's region: no new frame.
	a.redraw = false
	a.updateHover(6, 1)
	if a.redraw {
		t.Fatal("motion inside one widget must not request a frame")
	}

	// Move off onto the passive root: hover clears, frame requested, shape
	// reset (still emitted without a terminal in tests).
	a.updateHover(5, 8)
	if a.hover != nil || !a.redraw {
		t.Fatalf("hover = %#v redraw = %v", a.hover, a.redraw)
	}
	if a.pointerShape != "" {
		t.Fatalf("shape after move-off = %q", a.pointerShape)
	}
}
