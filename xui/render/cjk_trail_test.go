package render

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func TestWriteCellSkipsTrailAndContinuationColumn(t *testing.T) {
	r := NewRenderer()
	var buf strings.Builder
	dirty := []cell.DirtyCell{
		{X: 3, Y: 0, Cell: cell.Cell{Char: "翻", Width: 2}},
		{X: 4, Y: 0, Cell: cell.Cell{Char: " ", Width: 1, Trail: true}},
		{X: 5, Y: 0, Cell: cell.Cell{Char: "出", Width: 2}},
		// Mistaken non-Trail space on trail column of 出 — must still be skipped.
		{X: 6, Y: 0, Cell: cell.Cell{Char: " ", Width: 1}},
		{X: 7, Y: 0, Cell: cell.Cell{Char: "笔", Width: 2}},
	}
	if _, err := r.RenderDiff(&buf, dirty, -1, -1, false, 0); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "翻") || !strings.Contains(out, "出") || !strings.Contains(out, "笔") {
		t.Fatalf("missing CJK in output: %q", strings.ReplaceAll(out, "\x1b", "ESC"))
	}
	// After each wide glyph, a CUP to the trail column + space would wipe it.
	if strings.Contains(out, "翻\x1b[1;5H") || strings.Contains(out, "出\x1b[1;7H") {
		t.Fatalf("trail column write would wipe CJK: %q", strings.ReplaceAll(out, "\x1b", "ESC"))
	}
}
