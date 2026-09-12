package components

import (
	"testing"

	"github.com/pulseaiclub/xui"
)

func TestSurfaceRenderWideCharAfterFill(t *testing.T) {
	// ChatInput fills the body with non-default spaces then Print()'s CJK.
	// Rendering must not paint the leftover column under a width-2 glyph.
	screen := xui.NewScreen(10, 1)
	win := xui.NewWindow(screen)
	win.Clear()

	s := NewSurface(10, 1, nil)
	for x := range 10 {
		s.SetCell(x, 0, xui.Cell{Char: " ", Width: 1})
	}
	s.Print(0, 0, "中文", xui.Style{}, xui.WidthUnicode)
	s.Render(win)

	if got := screen.GetCell(0, 0); got.Char != "中" || got.Width != 2 {
		t.Fatalf("cell0 = %+v, want 中 width 2", got)
	}
	// Column 1 is the wide-char trail owned by screen.SetCell — not a second glyph.
	if got := screen.GetCell(1, 0); got.Char != " " || got.Width != 1 || !got.Trail {
		t.Fatalf("cell1 continuation = %+v", got)
	}
	if got := screen.GetCell(2, 0); got.Char != "文" || got.Width != 2 {
		t.Fatalf("cell2 = %+v, want 文 width 2", got)
	}
	// No gap: column 1 must not be an independent printable CJK / reverse block.
	if screen.GetCell(1, 0).Char == "中" || screen.GetCell(1, 0).Char == "文" {
		t.Fatal("continuation column overwritten with a real glyph")
	}
}

// TestSurfaceRenderClipsChildren ensures scrolled content cannot paint past the parent box
// (the MessageList / ScrollView leak into tui/footer).
func TestSurfaceRenderClipsChildren(t *testing.T) {
	screen := xui.NewScreen(20, 8)
	win := xui.NewWindow(screen)
	win.Clear()

	leak := NewSurface(18, 4, nil)
	leak.Print(0, 0, "AAAA", xui.Style{}, xui.WidthUnicode)
	leak.Print(0, 1, "BBBB", xui.Style{}, xui.WidthUnicode)
	leak.Print(0, 2, "CCCC", xui.Style{}, xui.WidthUnicode)
	leak.Print(0, 3, "DDDD", xui.Style{}, xui.WidthUnicode)

	list := Surface{
		Size:   Size{Width: 20, Height: 3},
		Widget: nil,
		Children: []SubSurface{{
			Origin:  Point{X: 0, Y: -1}, // scroll: first row off-screen, last row past list
			Surface: leak,
		}},
	}
	root := Surface{
		Size: Size{Width: 20, Height: 8},
		Children: []SubSurface{
			{Origin: Point{X: 0, Y: 0}, Surface: list},
			{Origin: Point{X: 0, Y: 3}, Surface: NewSurface(20, 5, nil)}, // tui zone
		},
	}
	root.Render(win)

	// Visible list rows: leak rows 1..2 → BBBB, CCCC at screen y=0,1
	if screen.GetCell(0, 0).Char != "B" {
		t.Fatalf("y0 want B got %q", screen.GetCell(0, 0).Char)
	}
	if screen.GetCell(0, 1).Char != "C" {
		t.Fatalf("y1 want C got %q", screen.GetCell(0, 1).Char)
	}
	// DDDD would be at list-local y=3 which is outside list height 3 — must not leak into tui.
	for y := 3; y < 8; y++ {
		ch := screen.GetCell(0, y).Char
		if ch == "D" {
			t.Fatalf("leaked D into row %d", y)
		}
	}
	// AAAA was above the clip (Y=-1) — must not appear.
	for y := range 8 {
		if screen.GetCell(0, y).Char == "A" {
			t.Fatalf("leaked A at row %d", y)
		}
	}
}

func TestCloneSurfaceDeepCopiesBuffers(t *testing.T) {
	child := NewSurface(2, 1, nil)
	child.Print(0, 0, "x", xui.Style{}, xui.WidthUnicode)
	original := Surface{
		Size: Size{Width: 2, Height: 1},
		Children: []SubSurface{{
			Surface: child,
		}},
	}

	clone := CloneSurface(original)
	clone.Children[0].Surface.Buffer[0].Char = "y"

	if got := original.Children[0].Surface.Buffer[0].Char; got != "x" {
		t.Fatalf("original child char = %q, want x", got)
	}
}

// TestSurfaceRenderResolvesDefaultColorsOnCanvas: text is painted with
// Fg-only styles all over the tree, and a cell that reaches the tty with a
// default background lets the terminal profile show under the glyph. The
// canvas the root carries is what those defaults resolve to, so a theme
// owns every cell it paints; explicit colors pass through untouched.
func TestSurfaceRenderResolvesDefaultColorsOnCanvas(t *testing.T) {
	screen := xui.NewScreen(6, 2)
	win := xui.NewWindow(screen)
	win.Clear()

	paper := xui.RGBColor(0xfa, 0xf9, 0xf5)
	ink := xui.RGBColor(0x14, 0x14, 0x13)
	red := xui.RGBColor(0xb4, 0x23, 0x18)
	root := NewSurface(6, 2, nil)
	root.Canvas = xui.Style{Fg: ink, Bg: paper}
	child := NewSurface(6, 1, nil)
	child.Print(0, 0, "ab", xui.Style{Fg: red}, xui.WidthUnicode)
	child.Print(2, 0, "c", xui.Style{}, xui.WidthUnicode)
	child.Print(3, 0, "d", xui.Style{Fg: red, Bg: red}, xui.WidthUnicode)
	root.Children = []SubSurface{{Surface: child}}
	root.Render(win)

	if got := screen.GetCell(0, 0).Style; !got.Fg.Equal(red) || !got.Bg.Equal(paper) {
		t.Fatalf("fg-only text = %+v, want red on paper", got)
	}
	if got := screen.GetCell(2, 0).Style; !got.Fg.Equal(ink) || !got.Bg.Equal(paper) {
		t.Fatalf("styleless text = %+v, want ink on paper", got)
	}
	if got := screen.GetCell(3, 0).Style; !got.Fg.Equal(red) || !got.Bg.Equal(red) {
		t.Fatalf("explicit colors = %+v, want red on red untouched", got)
	}
}

// A zero canvas keeps the terminal's own colors: the Terminal theme asks for
// exactly that, and a surface drawn outside a themed root loses nothing.
func TestSurfaceRenderWithoutCanvasKeepsTerminalColors(t *testing.T) {
	screen := xui.NewScreen(4, 1)
	win := xui.NewWindow(screen)
	win.Clear()

	s := NewSurface(4, 1, nil)
	s.Print(0, 0, "ab", xui.Style{}, xui.WidthUnicode)
	s.Render(win)

	if got := screen.GetCell(0, 0).Style; got.Fg.Kind != xui.ColorDefault || got.Bg.Kind != xui.ColorDefault {
		t.Fatalf("no canvas: style = %+v, want terminal defaults", got)
	}
}

// A child that carries its own canvas repaints its subtree on it, and the
// rest of the tree stays on the root's: an overlay pane can be its own page.
func TestSurfaceRenderChildCanvasOverridesTheRoot(t *testing.T) {
	screen := xui.NewScreen(4, 2)
	win := xui.NewWindow(screen)
	win.Clear()

	paper := xui.RGBColor(0xfa, 0xf9, 0xf5)
	night := xui.RGBColor(0x0a, 0x0a, 0x0a)
	root := NewSurface(4, 2, nil)
	root.Canvas = xui.Style{Bg: paper}
	root.Print(0, 0, "a", xui.Style{}, xui.WidthUnicode)
	pane := NewSurface(4, 1, nil)
	pane.Canvas = xui.Style{Bg: night}
	pane.Print(0, 0, "b", xui.Style{}, xui.WidthUnicode)
	root.Children = []SubSurface{{Origin: Point{Y: 1}, Surface: pane}}
	root.Render(win)

	if got := screen.GetCell(0, 0).Style.Bg; !got.Equal(paper) {
		t.Fatalf("root row bg = %+v, want paper", got)
	}
	if got := screen.GetCell(0, 1).Style.Bg; !got.Equal(night) {
		t.Fatalf("pane row bg = %+v, want night", got)
	}
}
