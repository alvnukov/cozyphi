package splash

import (
	"fmt"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

func TestSphereBigPreview(_ *testing.T) {
	sphere := &Sphere{Width: 52, Height: 30, Time: 0}
	surf := sphere.Draw(components.DrawContext{
		Max:    components.Size{Width: 52, Height: 30},
		Method: xui.WidthUnicode,
	})
	for y := 0; y < surf.Size.Height; y++ {
		line := make([]byte, 0, surf.Size.Width)
		for x := 0; x < surf.Size.Width; x++ {
			c := surf.Buffer[y*surf.Size.Width+x]
			if c.Char == "" {
				line = append(line, ' ')
			} else {
				line = append(line, c.Char[0])
			}
		}
		fmt.Println(string(line))
	}
}
