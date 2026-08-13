package splash

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

func TestSphereDrawFillsEllipse(t *testing.T) {
	sphere := &Sphere{Width: 20, Height: 20, Time: 0.5}
	surf := sphere.Draw(components.DrawContext{
		Max:    components.Size{Width: 20, Height: 20},
		Method: xui.WidthUnicode,
	})
	if surf.Size.Width != 20 || surf.Size.Height != 20 {
		t.Fatalf("size = %dx%d", surf.Size.Width, surf.Size.Height)
	}
	nonEmpty := 0
	for _, c := range surf.Buffer {
		if c.Char != "" && c.Char != " " {
			nonEmpty++
		}
	}
	if nonEmpty < 40 {
		t.Fatalf("expected sphere cells, got %d non-empty", nonEmpty)
	}
}
