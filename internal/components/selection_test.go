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
