package status

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
)

func TestExpandable(t *testing.T) {
	ex := &Expandable{
		Title:      &layout.Text{Content: "Title"},
		Child:      &layout.Text{Content: "Body"},
		Expandable: true,
		Expanded:   true,
		Theme:      components.DefaultTheme(),
	}
	s := ex.Draw(components.DrawContext{Max: components.Size{Width: 30, Height: 10}})
	if s.Size.Height < 2 {
		t.Fatalf("expandable height %d", s.Size.Height)
	}
}

func TestToolHeaderSpinner(t *testing.T) {
	sp := NewSpinner(xui.Style{})
	sp.Tick()
	h := &ToolHeader{Name: "bash", Detail: "ls", Status: ToolRunning, Spinner: sp, Theme: components.DefaultTheme()}
	s := h.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 1}})
	if s.Size.Height < 1 {
		t.Fatal("empty tool header")
	}
}

func TestSpinnerGlyphs(t *testing.T) {
	sp := NewSpinner(xui.Style{})
	glyphs := map[string]struct{}{}
	scans := map[string]struct{}{}
	for i := 0; i < 20; i++ {
		g := sp.Glyph()
		if g == "" || xui.StringWidth(g, xui.WidthUnicode) != 1 {
			t.Fatalf("frame %d glyph %q width %d", i, g, xui.StringWidth(g, xui.WidthUnicode))
		}
		sc := sp.Scan()
		if xui.StringWidth(sc, xui.WidthUnicode) != scanW {
			t.Fatalf("frame %d scan %q width %d want %d", i, sc, xui.StringWidth(sc, xui.WidthUnicode), scanW)
		}
		glyphs[g] = struct{}{}
		scans[sc] = struct{}{}
		sp.Tick()
	}
	if len(glyphs) != 10 {
		t.Fatalf("want 10 unique braille frames, got %d", len(glyphs))
	}
	if len(scans) < 6 {
		t.Fatalf("scan bar did not bounce, got %d frames", len(scans))
	}
}
