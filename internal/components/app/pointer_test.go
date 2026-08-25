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

func TestPointerShapeAtAsksDeepestWidget(t *testing.T) {
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

	if got := pointerShapeAt(root, 5, 1); got != components.ShapePointer {
		t.Fatalf("title row shape = %q", got)
	}
	if got := pointerShapeAt(root, 5, 3); got != components.ShapeText {
		t.Fatalf("body row shape = %q", got)
	}
	if got := pointerShapeAt(root, 5, 8); got != "" {
		t.Fatalf("plain root shape = %q", got)
	}
	if got := pointerShapeAt(root, 50, 50); got != "" {
		t.Fatalf("miss shape = %q", got)
	}
}
