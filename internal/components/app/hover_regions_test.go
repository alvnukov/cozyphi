package app

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
)

type regionStub struct{ shapeStub }

func (*regionStub) HoverRegion(x, _ int) int { return x/5 + 1 }

func (s *regionStub) Draw(components.DrawContext) components.Surface {
	return components.Surface{Size: components.Size{Width: 10, Height: 2}, Widget: s}
}

func TestHoverRegionsAvoidRedundantFrames(t *testing.T) {
	stub := &regionStub{shapeStub{shape: components.ShapePointer}}
	a := &App{lastSurf: stub.Draw(components.DrawContext{})}
	a.updateHover(1, 0)
	a.redraw = false
	a.updateHover(2, 0)
	if a.redraw || a.hover.X != 2 {
		t.Fatal("motion within one control must retain coordinates without repainting")
	}
	a.updateHover(6, 0)
	if !a.redraw {
		t.Fatal("moving to another control must repaint")
	}
	a.redraw = false
	a.updateHover(6, 0)
	if a.redraw {
		t.Fatal("stationary pointer must remain idle")
	}
}

func TestHoverReconcilesClosedOverlayWithoutMouseMotion(t *testing.T) {
	stub := &regionStub{shapeStub{shape: components.ShapePointer}}
	a := &App{lastSurf: stub.Draw(components.DrawContext{})}
	a.updateHover(1, 0)
	a.redraw = false
	if !a.refreshHover((&plainStub{}).Draw(components.DrawContext{})) {
		t.Fatal("closing the hovered overlay must invalidate its hover")
	}
	if a.hover != nil || a.pointerShape != "" {
		t.Fatal("closed overlay retained the pointer hand or hover")
	}
	a.redraw = false
	if a.refreshHover(a.lastSurf) || a.redraw {
		t.Fatal("unchanged frame must stay idle")
	}
}
