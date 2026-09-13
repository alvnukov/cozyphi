package tooltip

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func place(t *testing.T, text string, at components.Point) (components.SubSurface, string) {
	t.Helper()
	sub, ok := PlaceTooltip(text, at, 80, 24, components.DefaultTheme(), xui.WidthUnicode)
	if !ok {
		t.Fatalf("PlaceTooltip(%q) refused a roomy screen", text)
	}
	return sub, components.SurfaceText(sub.Surface)
}

// The hint floats above and right of the pointer, in the shared panel
// chrome: solid fill, rounded border, first line foreground.
func TestPlaceTooltipAnchorsAboveRight(t *testing.T) {
	sub, text := place(t, "Run the command", components.Point{X: 10, Y: 10})
	if sub.Origin.X != 12 || sub.Origin.Y != 10-sub.Surface.Size.Height-1 {
		t.Fatalf("origin = %v, want right of and above the pointer", sub.Origin)
	}
	if !strings.Contains(text, "Run the command") {
		t.Fatalf("hint text missing: %q", text)
	}
	if !strings.Contains(text, "╭") {
		t.Fatalf("hint must wear the rounded border: %q", text)
	}
	if sub.Surface.Widget != nil {
		t.Fatalf("hint panel must not hit-test: clicks fall through")
	}
}

// A pointer near the top flips the panel below; the right edge shifts the
// panel back inside the screen.
func TestPlaceTooltipFlipsAtEdges(t *testing.T) {
	sub, _ := place(t, "Run the command", components.Point{X: 10, Y: 0})
	if sub.Origin.Y != 1 {
		t.Fatalf("top edge must flip below: %v", sub.Origin)
	}
	if sub.Origin.X+sub.Surface.Size.Width > 80 {
		t.Fatalf("panel leaves the screen: %v", sub.Origin)
	}

	sub, _ = place(t, "Run the command", components.Point{X: 79, Y: 10})
	if sub.Origin.X+sub.Surface.Size.Width > 80 || sub.Origin.X < 0 {
		t.Fatalf("right edge must shift inside: %v", sub.Origin)
	}
}

// Long text wraps on words, honors explicit newlines and is cut with an
// ellipsis once it outgrows the line or height budget.
func TestPlaceTooltipWrapsAndCaps(t *testing.T) {
	// 60-cell sentence wraps into lines no wider than the cap.
	sub, text := place(t, "approve the plan and run the tool once with a bound budget",
		components.Point{X: 10, Y: 12})
	for line := range strings.SplitSeq(text, "\n") {
		if w := xui.StringWidth(strings.TrimRight(line, " "), xui.WidthUnicode); w > TooltipMaxWidth+2 {
			t.Fatalf("wrapped line %q is %d cells, cap %d", line, w, TooltipMaxWidth)
		}
	}

	// Explicit newlines start fresh lines.
	_, text = place(t, "first line\nsecond line", components.Point{X: 10, Y: 12})
	if !strings.Contains(text, "first") || !strings.Contains(text, "second") {
		t.Fatalf("newline hint lost a line: %q", text)
	}

	// A wall of words cannot outgrow the height budget.
	sub, _ = place(t, strings.Repeat("word ", 100), components.Point{X: 10, Y: 12})
	if sub.Surface.Size.Height > 6+2 {
		t.Fatalf("hint height = %d, want at most the line cap plus frame", sub.Surface.Size.Height)
	}

	// A word wider than the line wraps at the glyph level.
	sub, _ = place(t, strings.Repeat("u", 200), components.Point{X: 10, Y: 12})
	if sub.Surface.Size.Width > TooltipMaxWidth+2 {
		t.Fatalf("unbreakable word inflated the panel to %d cells", sub.Surface.Size.Width)
	}
}

// A screen too small to hold a hint refuses instead of corrupting it.
func TestPlaceTooltipRefusesTinyScreens(t *testing.T) {
	if _, ok := PlaceTooltip(
		"hint",
		components.Point{X: 1, Y: 1},
		10,
		2,
		components.DefaultTheme(),
		xui.WidthUnicode,
	); ok {
		t.Fatalf("a 10x2 screen cannot hold a framed hint")
	}
	if _, ok := PlaceTooltip(
		"hint",
		components.Point{X: 1, Y: 1},
		8,
		24,
		components.DefaultTheme(),
		xui.WidthUnicode,
	); ok {
		t.Fatalf("a screen narrower than the minimum wrap width must refuse")
	}
}

// The hint paints above other layers and never intercepts a hit test:
// z-order puts it last, and a miss through it finds the widget underneath.
func TestPlaceTooltipFloatsAboveAndClicksFallThrough(t *testing.T) {
	under := (&stub{}).Draw(components.DrawContext{})
	root := components.NewSurface(80, 24, nil)
	root.Children = []components.SubSurface{{Origin: components.Point{X: 0, Y: 0}, Surface: under}}
	sub, _ := place(t, "hint", components.Point{X: 3, Y: 3})
	root.Children = append(root.Children, sub)
	if got := root.HitTest(4, 4); got != under.Widget {
		t.Fatalf("hit through the hint = %v, want the control underneath", got)
	}
	if root.Children[len(root.Children)-1].Z <= root.Children[0].Z {
		t.Fatalf("the hint must sort above the frame")
	}
}

type stub struct{}

func (*stub) Handle(*components.EventContext, xui.Event) {}

func (s *stub) Draw(components.DrawContext) components.Surface {
	return components.Surface{Size: components.Size{Width: 80, Height: 24}, Widget: s}
}
