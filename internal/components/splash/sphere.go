package splash

import (
	"math"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

// Classic sphere charset.
const sphereCharset = " .:-=+*#%@"

// Default sphere palette endpoints (dark green → bright green).
var (
	spherePrimary   = rgb{0, 55, 0}
	sphereSecondary = rgb{0, 255, 136}
)

type rgb struct{ r, g, b uint8 }

func lerpRGB(a, b rgb, t float64) rgb {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return rgb{
		r: uint8(math.Round(float64(a.r) + float64(b.r-a.r)*t)),
		g: uint8(math.Round(float64(a.g) + float64(b.g-a.g)*t)),
		b: uint8(math.Round(float64(a.b) + float64(b.b-a.b)*t)),
	}
}

// Sphere is an ASCII sphere for the splash screen.
// Drive animation by advancing Time (seconds) each frame with App.Anim.
type Sphere struct {
	Width  int // default 40
	Height int // default 40
	Time   float64
	// Fast speeds noise scroll (1.8x when true, else 1x).
	Fast bool

	noise   glowNoise
	palette []xui.Color
}

func (o *Sphere) ensure() {
	if o.Width <= 0 {
		o.Width = 40
	}
	if o.Height <= 0 {
		o.Height = 40
	}
	if o.noise.seed == 0 {
		o.noise = newGlowNoise(2654435761)
	}
	if len(o.palette) == 0 {
		const n = 64
		o.palette = make([]xui.Color, n)
		for i := range n {
			c := lerpRGB(spherePrimary, sphereSecondary, float64(i)/float64(n-1))
			o.palette[i] = xui.RGBColor(c.r, c.g, c.b)
		}
	}
}

// Handle is a no-op; the sphere animates via Time, not input.
func (*Sphere) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the noise-lit ASCII sphere into a surface.
func (o *Sphere) Draw(ctx components.DrawContext) components.Surface {
	o.ensure()
	w, h := o.Width, o.Height
	if maxW := ctx.Max.Width; maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH := ctx.Max.Height; maxH > 0 && h > maxH {
		h = maxH
	}
	if w < 3 {
		w = 3
	}
	if h < 3 {
		h = 3
	}

	s := components.NewSurface(w, h, o)

	// Aspect correction: terminal cells are roughly twice as tall as wide.
	const kx = 0.5
	cx := float64(w) / 2
	cy := float64(h) / 2
	rx := math.Max(1, float64(w)/2-1)
	ry := math.Max(1, float64(h)/(2*kx)-1)
	radius := math.Min(rx, ry)
	r2 := radius * radius
	invR2 := 1 / r2
	invKx := 1 / kx

	speed := 1.0
	if o.Fast {
		speed = 1.8
	}
	chars := []rune(sphereCharset)
	nChars := len(chars)
	nPal := len(o.palette)

	for row := 0; row < h; row++ {
		py := (float64(row) - cy) * invKx
		py2 := py * py
		if py2 >= r2 {
			continue
		}
		half := math.Sqrt(r2 - py2)
		x0 := int(math.Max(0, math.Floor(cx-half)))
		x1 := int(math.Min(float64(w-1), math.Ceil(cx+half)))
		for col := x0; col <= x1; col++ {
			px := float64(col) - cx
			d2 := px*px + py2
			if d2 >= r2 {
				continue
			}
			falloff := 1 - d2*invR2
			g := o.noise.sample(float64(col), float64(row), o.Time, speed) * falloff
			if g <= 0 {
				continue
			}
			gi := int(math.Min(float64(nChars-1), math.Floor(g*float64(nChars))))
			pi := int(math.Min(float64(nPal-1), math.Floor(g*float64(nPal))))
			s.SetCell(col, row, xui.Cell{
				Char:  string(chars[gi]),
				Width: 1,
				Style: xui.Style{Fg: o.palette[pi]},
			})
		}
	}
	return s
}
