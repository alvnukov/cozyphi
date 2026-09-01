package components

import (
	"testing"

	"github.com/pulseaiclub/xui"
)

// The wave is a brightness sweep, not a rewrite: one span per rune, the text
// intact, every foreground a blend between base and accent.
func TestWaveLabelKeepsTheTextAndBlends(t *testing.T) {
	base := xui.Style{Fg: xui.RGBColor(0x60, 0x60, 0x60)}
	accent := xui.Style{Fg: xui.RGBColor(0xff, 0xff, 0xff)}
	spans := WaveLabel("Generating", accent, base)
	if len(spans) != len([]rune("Generating")) {
		t.Fatalf("one span per rune: got %d", len(spans))
	}
	text := ""
	varied := false
	for _, sp := range spans {
		text += sp.Text
		if sp.Style.Fg != base.Fg {
			varied = true
		}
		if sp.Style.Bg != base.Bg || sp.Style.Bold != base.Bold {
			t.Fatalf("the wave changes brightness only, got %+v", sp.Style)
		}
	}
	if text != "Generating" {
		t.Fatalf("label text survives the wave: %q", text)
	}
	if !varied {
		t.Fatal("at least one letter sits brighter than the base")
	}
}

// A non-truecolor theme cannot blend; the band head falls back to a hard
// accent instead of vanishing.
func TestWaveLabelFallsBackWithoutTruecolor(t *testing.T) {
	base := xui.Style{Fg: xui.IndexedColor(7)}
	accent := xui.Style{Fg: xui.IndexedColor(12)}
	spans := WaveLabel("abc", accent, base)
	accented := 0
	for _, sp := range spans {
		if sp.Style.Fg == accent.Fg {
			accented++
		}
	}
	if accented != 1 {
		t.Fatalf("exactly the band head wears the accent, got %d", accented)
	}
}

func TestPulseStyleBreathesBetweenBaseAndAccent(t *testing.T) {
	base := xui.Style{Fg: xui.RGBColor(0x40, 0x40, 0x40)}
	accent := xui.Style{Fg: xui.RGBColor(0xc0, 0xc0, 0xc0)}
	st := PulseStyle(accent, base)
	if st.Fg.Kind != xui.ColorRGB {
		t.Fatalf("truecolor pulse blends: %+v", st.Fg)
	}
	if st.Fg.R < 0x40 || st.Fg.R > 0xc0 {
		t.Fatalf("the pulse stays between base and accent, got %#x", st.Fg.R)
	}
	ansi := PulseStyle(xui.Style{Fg: xui.IndexedColor(12)}, xui.Style{Fg: xui.IndexedColor(7)})
	if ansi.Fg != xui.IndexedColor(12) {
		t.Fatalf("without truecolor the glyph holds the accent, got %+v", ansi.Fg)
	}
}
