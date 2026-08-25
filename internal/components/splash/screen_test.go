package splash

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestWordmarkUniformWidth(t *testing.T) {
	want := 0
	for i, row := range wordmark {
		got := xui.StringWidth(row, xui.WidthUnicode)
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("wordmark row %d width = %d, want %d (rows must align)", i, got, want)
		}
	}
	if want < 40 || want > 78 {
		t.Fatalf("wordmark width = %d, want within [40, 78] so it fits an 80-col terminal", want)
	}
}

func drawText(t *testing.T, s Screen, w, h int) string {
	t.Helper()
	surf := s.Draw(components.DrawContext{
		Max:    components.Size{Width: w, Height: h},
		Method: xui.WidthUnicode,
	})
	return surfText(surf)
}

// surfText renders a surface tree into viewable text with children placed
// at their origins, so layout assertions see what the user sees.
func surfText(root components.Surface) string {
	canvas := components.NewSurface(root.Size.Width, root.Size.Height, nil)
	if root.Buffer != nil {
		copy(canvas.Buffer, root.Buffer)
	}
	for _, ch := range root.Children {
		for y := 0; y < ch.Surface.Size.Height; y++ {
			for x := 0; x < ch.Surface.Size.Width; x++ {
				c := ch.Surface.Buffer[y*ch.Surface.Size.Width+x]
				canvas.SetCell(ch.Origin.X+x, ch.Origin.Y+y, c)
			}
		}
	}
	return components.SurfaceText(canvas)
}

// hasWordmark reports whether any wordmark row appears in the given text.
func hasWordmark(text string) bool {
	for _, row := range wordmark {
		if row = strings.TrimSpace(row); row != "" && strings.Contains(text, row) {
			return true
		}
	}
	return false
}

func TestScreenDrawShowsLogoAndCopy(t *testing.T) {
	s := Screen{Theme: components.DefaultTheme(), Version: "v9.9.9"}
	surf := s.Draw(components.DrawContext{
		Max:    components.Size{Width: 100, Height: 40},
		Method: xui.WidthUnicode,
	})
	if surf.Size.Width != 100 || surf.Size.Height != 40 {
		t.Fatalf("size = %dx%d, want 100x40", surf.Size.Width, surf.Size.Height)
	}
	text := surfText(surf)
	if !hasWordmark(text) {
		t.Error("expected the wordmark on a wide terminal")
	}
	for _, want := range []string{"terminal coding agent", "v9.9.9", "Ctrl+K", "Type a message below"} {
		if !strings.Contains(text, want) {
			t.Errorf("splash text missing %q", want)
		}
	}
}

func TestScreenCentered(t *testing.T) {
	s := Screen{Theme: components.DefaultTheme()}
	surf := s.Draw(components.DrawContext{
		Max:    components.Size{Width: 100, Height: 40},
		Method: xui.WidthUnicode,
	})
	minX, maxX := -1, -1
	for y := 0; y < surf.Size.Height; y++ {
		for x := 0; x < surf.Size.Width; x++ {
			c := surf.Buffer[y*surf.Size.Width+x]
			if c.Char == "" || c.Char == " " {
				continue
			}
			if minX == -1 || x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
		}
	}
	if minX == -1 {
		t.Fatal("splash painted nothing")
	}
	left, right := minX, surf.Size.Width-1-maxX
	if abs(left-right) > 2 {
		t.Errorf("content off-center: left margin %d, right margin %d", left, right)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestScreenNarrowFallsBackToPlainText(t *testing.T) {
	s := Screen{Theme: components.DefaultTheme()}
	surf := s.Draw(components.DrawContext{
		Max:    components.Size{Width: 40, Height: 40},
		Method: xui.WidthUnicode,
	})
	text := surfText(surf)
	if hasWordmark(text) {
		t.Error("narrow terminal should not draw the wordmark")
	}
	if !strings.Contains(text, "CozyPhi") {
		t.Errorf("narrow fallback missing brand text:\n%s", text)
	}
}

// TestScreenPreview prints the welcome screen for eyeballing; run with
// go test ./internal/components/splash/ -run TestScreenPreview -v.
func TestScreenPreview(t *testing.T) {
	s := Screen{Theme: components.DefaultTheme(), Version: "v0.16.0"}
	fmt.Println(drawText(t, s, 90, 20))
}
