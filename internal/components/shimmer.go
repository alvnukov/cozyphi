package components

import (
	"math"
	"time"

	"github.com/pulseaiclub/xui"
)

// The soft claude-code-style activity animation: letters shimmer, a glyph
// breathes. Both are driven by the wall clock, so the motion is
// fps-independent and every site pulses in the same rhythm.

const (
	waveTick    = 80 * time.Millisecond
	pulsePeriod = 1200 * time.Millisecond
)

// WaveLabel renders a label with a time-driven bright band sweeping across
// its letters: a per-rune blend from base toward accent, brightness only —
// no color change, no blinking.
func WaveLabel(label string, accent, base xui.Style) []Span {
	letters := []rune(label)
	n := len(letters)
	if n == 0 {
		return nil
	}
	phase := int(time.Now().UnixNano()/int64(waveTick)) % n
	out := make([]Span, 0, n)
	for i, r := range letters {
		dist := ((i-phase)%n + n) % n
		t := (math.Cos(float64(dist)/float64(n)*2*math.Pi) + 1) / 2
		st := base
		if c, ok := blendColor(base.Fg, accent.Fg, t); ok {
			st.Fg = c
		} else if dist == 0 {
			st = accent
		}
		out = append(out, Span{Text: string(r), Style: st})
	}
	return out
}

// PulseStyle breathes a single glyph: the base style's foreground eases
// toward the accent and back on a slow cosine. Callers with a non-truecolor
// theme get the accent held steady — a calm mark beats a hard blink.
func PulseStyle(accent, base xui.Style) xui.Style {
	phase := float64(time.Now().UnixNano()%int64(pulsePeriod)) / float64(pulsePeriod)
	t := (math.Cos(phase*2*math.Pi) + 1) / 2
	st := base
	if c, ok := blendColor(base.Fg, accent.Fg, t); ok {
		st.Fg = c
		return st
	}
	return accent
}

// blendColor linearly mixes two RGB colors by t (clamped to [0,1]). It
// returns false when either color is not truecolor, so callers fall back to
// a hard highlight.
func blendColor(a, b xui.Color, t float64) (xui.Color, bool) {
	if a.Kind != xui.ColorRGB || b.Kind != xui.ColorRGB {
		return xui.Color{}, false
	}
	if t <= 0 {
		return a, true
	}
	if t >= 1 {
		return b, true
	}
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return xui.RGBColor(lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)), true
}
