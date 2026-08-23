package screen

import (
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func TestSetCellMarksTrail(t *testing.T) {
	s := NewScreen(6, 1)
	s.SetCell(0, 0, cell.Cell{Char: "笔", Width: 2})
	if got := s.GetCell(0, 0); got.Char != "笔" || got.Width != 2 || got.Trail {
		t.Fatalf("primary = %+v", got)
	}
	if got := s.GetCell(1, 0); !got.Trail || got.Char != " " || got.Width != 1 {
		t.Fatalf("trail = %+v", got)
	}
}

func TestDiffNeverEmitsTrail(t *testing.T) {
	s := NewScreen(6, 1)
	s.SetCell(0, 0, cell.Cell{Char: "笔", Width: 2})
	s.SetCell(2, 0, cell.Cell{Char: "记", Width: 2})
	s.MarkRefresh()
	for _, d := range s.Diff() {
		if d.Cell.Trail {
			t.Fatalf("Diff emitted trail at x=%d", d.X)
		}
		if d.Cell.Char == " " && d.X == 1 {
			t.Fatal("Diff emitted continuation space at x=1")
		}
	}
}

func TestDiffColumnByColumnBackBufferDoesNotEmitTrails(t *testing.T) {
	// Simulate a renderer painting every non-default column including old-style
	// continuation spaces that are now marked Trail.
	s := NewScreen(8, 1)
	win := NewWindow(s)
	for x, c := range []cell.Cell{
		{Char: "1", Width: 1},
		{Char: ".", Width: 1},
		{Char: " ", Width: 1},
		{Char: "翻", Width: 2},
		{Char: " ", Width: 1, Trail: true},
		{Char: "出", Width: 2},
		{Char: " ", Width: 1, Trail: true},
	} {
		if c.Trail {
			continue // correct paint skips trails
		}
		win.SetCell(x, 0, c)
	}
	s.MarkRefresh()
	for _, d := range s.Diff() {
		if d.Cell.Trail {
			t.Fatalf("emitted trail %+v", d)
		}
	}
	if got := s.GetCell(3, 0); got.Char != "翻" || got.Width != 2 {
		t.Fatalf("primary 翻 = %+v", got)
	}
	if got := s.GetCell(4, 0); !got.Trail {
		t.Fatalf("expected trail at 4, got %+v", got)
	}
}
