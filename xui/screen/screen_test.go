package screen

import (
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func TestCellEqual(t *testing.T) {
	a := cell.EmptyCell()
	b := cell.EmptyCell()
	if !a.Equal(b) {
		t.Fatal("empty cells should be equal")
	}
	a = cell.Cell{Char: "中", Width: 2, Style: cell.Style{Bold: true}}
	b = cell.Cell{Char: "中", Width: 2, Style: cell.Style{Bold: true}}
	if !a.Equal(b) {
		t.Fatal("identical cells should be equal")
	}
	b.Style.Bold = false
	if a.Equal(b) {
		t.Fatal("different styles should not be equal")
	}
}

func TestScreenDiffAndPresent(t *testing.T) {
	s := NewScreen(4, 2)
	dirty := s.Diff()
	if len(dirty) != 8 {
		t.Fatalf("full refresh expected 8 cells, got %d", len(dirty))
	}
	s.Present()

	s.Clear()
	dirty = s.Diff()
	if len(dirty) != 0 {
		t.Fatalf("expected no dirty cells, got %d", len(dirty))
	}

	s.SetCell(1, 0, cell.Cell{Char: "A", Width: 1, Style: cell.Style{Bold: true}})
	dirty = s.Diff()
	// Row-granular damage: one changed cell dirties the whole row (width=4).
	if len(dirty) != 4 {
		t.Fatalf("expected full dirty row (4 cells), got %d", len(dirty))
	}
	found := false
	for _, d := range dirty {
		if d.X == 1 && d.Y == 0 && d.Cell.Char == "A" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing dirty cell A at (1,0): %+v", dirty)
	}
	s.Present()

	s.Clear()
	s.SetCell(1, 0, cell.Cell{Char: "A", Width: 1, Style: cell.Style{Bold: true}})
	dirty = s.Diff()
	if len(dirty) != 0 {
		t.Fatalf("unchanged cell should not be dirty, got %d", len(dirty))
	}
}

func TestWideCharDeleteDamagesBothColumns(t *testing.T) {
	s := NewScreen(4, 1)
	_ = s.Diff()
	s.Present()

	s.Clear()
	s.SetCell(0, 0, cell.Cell{Char: "宽", Width: 2})
	s.SetCell(2, 0, cell.Cell{Char: "字", Width: 2})
	_ = s.Diff()
	s.Present()

	// Delete first wide glyph → two ASCII cells must both be in the damage region.
	s.Clear()
	s.SetCell(0, 0, cell.Cell{Char: "A", Width: 1})
	s.SetCell(1, 0, cell.Cell{Char: "B", Width: 1})
	s.SetCell(2, 0, cell.Cell{Char: "字", Width: 2})
	dirty := s.Diff()
	seen := map[int]bool{}
	for _, d := range dirty {
		seen[d.X] = true
	}
	if !seen[0] || !seen[1] {
		t.Fatalf("expected damage on cols 0 and 1 after wide→narrow, got %v", dirty)
	}
}

func TestDiffRowDamageClearsUnchangedColumns(t *testing.T) {
	// Scroll-style update: one column changes, but Diff must still re-emit the
	// rest of the row so TTY ghosts in untouched columns get overwritten.
	s := NewScreen(6, 1)
	_ = s.Diff()
	s.Present()

	s.Clear()
	s.SetCell(0, 0, cell.Cell{Char: ")", Width: 1})
	s.SetCell(2, 0, cell.Cell{Char: "s", Width: 1})
	s.SetCell(4, 0, cell.Cell{Char: "d", Width: 1})
	_ = s.Diff()
	s.Present()

	s.Clear()
	s.SetCell(0, 0, cell.Cell{Char: "中", Width: 2})
	s.SetCell(2, 0, cell.Cell{Char: "文", Width: 2})
	dirty := s.Diff()
	seen := map[int]string{}
	for _, d := range dirty {
		seen[d.X] = d.Cell.Char
	}
	// Whole row rewritten — blanks at old ghost columns must be present.
	if _, ok := seen[4]; !ok {
		t.Fatalf("expected blank/clear emit at col 4 to wipe ghost 'd', got %v", dirty)
	}
	if seen[0] != "中" || seen[2] != "文" {
		t.Fatalf("unexpected row content: %v", seen)
	}
}

func TestWindowPrint(t *testing.T) {
	s := NewScreen(20, 5)
	w := NewWindow(s)
	n := w.Print(0, 0, "hi", cell.Style{Bold: true})
	if n != 2 {
		t.Fatalf("print width = %d", n)
	}
	c := w.GetCell(0, 0)
	if c.Char != "h" || !c.Style.Bold {
		t.Fatalf("cell = %+v", c)
	}
	child := w.Child(2, 1, 5, 2)
	child.Print(0, 0, "x", cell.Style{})
	if s.GetCell(2, 1).Char != "x" {
		t.Fatal("child write missed")
	}
}
