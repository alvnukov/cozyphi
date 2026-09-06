package components

import (
	"testing"

	"github.com/pulseaiclub/xui"
)

func TestExtractSurfaceText(t *testing.T) {
	s := NewSurface(10, 3, nil)
	s.Print(0, 0, "hello", xui.Style{}, xui.WidthUnicode)
	s.Print(0, 1, "world", xui.Style{}, xui.WidthUnicode)
	got := ExtractSurfaceText(s, 0, 0, 4, 1)
	if got != "hello\nworld" {
		t.Fatalf("got %q", got)
	}
	partial := ExtractSurfaceText(s, 1, 0, 3, 0)
	if partial != "ell" {
		t.Fatalf("partial=%q", partial)
	}
}

func TestExtractSurfaceTextCJKNoContinuationSpaces(t *testing.T) {
	// SetCell pads wide glyphs with Width=1 " " trail cells; copy must not
	// turn those into "二 进 制".
	s := NewSurface(20, 1, nil)
	s.Print(0, 0, "二进制文件 cozyphi", xui.Style{}, xui.WidthUnicode)
	got := ExtractSurfaceText(s, 0, 0, 19, 0)
	want := "二进制文件 cozyphi"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Selecting only the trail half of the first glyph still yields the rune.
	half := ExtractSurfaceText(s, 1, 0, 1, 0)
	if half != "二" {
		t.Fatalf("half-select=%q, want 二", half)
	}
}

func TestExtractSurfaceTextSkipsRuleChrome(t *testing.T) {
	s := NewSurface(20, 1, nil)
	s.SetCell(0, 0, xui.Cell{Char: "▎", Width: 1})
	s.SetCell(1, 0, xui.Cell{Char: " ", Width: 1})
	s.Print(2, 0, "你好", xui.Style{}, xui.WidthUnicode)
	got := ExtractSurfaceText(s, 0, 0, 19, 0)
	if got != "你好" {
		t.Fatalf("got %q, want 你好", got)
	}
}

// TestExtractSurfaceTextSkipsMarkedChrome: marked cells leave the clipboard
// whole — numbers, markers and the pad columns between them go, and the
// content keeps its own leading indentation, which no trim may eat.
func TestExtractSurfaceTextSkipsMarkedChrome(t *testing.T) {
	s := NewSurface(20, 2, nil)
	s.Print(0, 0, " 12 + ", xui.Style{}, xui.WidthUnicode)
	s.Print(6, 0, "  indented", xui.Style{}, xui.WidthUnicode)
	s.Print(0, 1, " 13   ", xui.Style{}, xui.WidthUnicode)
	s.Print(6, 1, "kept", xui.Style{}, xui.WidthUnicode)
	MarkChrome(&s, 0, 0, 6)
	MarkChrome(&s, 0, 1, 6)

	got := ExtractSurfaceText(s, 0, 0, 19, 1)
	if want := "  indented\nkept"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestExtractSurfaceTextChromeMaskSurvivesNesting: the mask is composited
// with the cells, so a child's chrome stays chrome at the parent's origin and
// a child's text painted over a parent's chrome comes back as text.
func TestExtractSurfaceTextChromeMaskSurvivesNesting(t *testing.T) {
	child := NewSurface(10, 1, nil)
	child.Print(0, 0, "# code", xui.Style{}, xui.WidthUnicode)
	MarkChrome(&child, 0, 0, 2)

	parent := NewSurface(12, 3, nil)
	parent.Print(0, 2, "xxxx", xui.Style{}, xui.WidthUnicode)
	MarkChrome(&parent, 0, 2, 4)
	parent.Children = append(parent.Children, SubSurface{
		Origin:  Point{X: 1, Y: 2},
		Surface: child,
	})

	// "code" starts on a column the parent marked: the child's text wins, so
	// dropping the first letter would mean the mask outlived the cell.
	got := ExtractSurfaceText(parent, 0, 2, 11, 2)
	if want := "code"; got != want {
		t.Fatalf("nested mask: got %q, want %q", got, want)
	}
}

// TestSelectionHighlightLeavesChromeAlone: the tint stops at the text, so a
// drag over a diff lights the code column and not the numbers beside it.
func TestSelectionHighlightLeavesChromeAlone(t *testing.T) {
	s := NewSurface(6, 1, nil)
	s.Print(0, 0, "12 ab", xui.Style{}, xui.WidthUnicode)
	MarkChrome(&s, 0, 0, 3)

	bg := xui.Style{Bg: xui.IndexedColor(4)}
	ApplySelectionHighlight(&s, 0, 0, 5, 0, bg)
	for x := range 3 {
		if s.Buffer[x].Style.Bg == bg.Bg {
			t.Fatalf("chrome cell %d took the selection tint", x)
		}
	}
	for x := 3; x < 5; x++ {
		if s.Buffer[x].Style.Bg != bg.Bg {
			t.Fatalf("text cell %d missed the selection tint", x)
		}
	}
}

func TestInTextSelection(t *testing.T) {
	if !InTextSelection(2, 0, 0, 0, 5, 0) {
		t.Fatal("mid single line")
	}
	if InTextSelection(0, 1, 2, 0, 5, 0) {
		t.Fatal("below single line")
	}
	if !InTextSelection(0, 1, 2, 0, 3, 2) {
		t.Fatal("middle of multi-line")
	}
}
