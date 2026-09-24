package render

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func row(y int, text string) []cell.DirtyCell {
	var out []cell.DirtyCell
	x := 0
	for s := text; s != ""; {
		cl, w, rest := cell.FirstGrapheme(s, cell.WidthUnicode)
		out = append(out, cell.DirtyCell{X: x, Y: y, Cell: cell.Cell{Char: cl, Width: uint8(w)}})
		x += w
		s = rest
	}
	return out
}

// A painted frame must never let a write wrap onto the next row: when the
// terminal draws a glyph at a width the model did not measure (some
// terminals render ambiguous-width or unjoined emoji clusters wider or
// narrower), the row's last cell would
// otherwise land in column 0 of the row below — a row this frame did not
// repaint, so the ghost stays.
func TestRenderDiffPaintsWithAutowrapOff(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := r.RenderDiff(&buf, row(0, "✅ ok"), 0, 1, true, 0); err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	off := strings.Index(out, seqAutowrapReset)
	on := strings.LastIndex(out, seqAutowrapSet)
	glyph := strings.Index(out, "✅")
	if off < 0 || on < 0 || off > glyph || on < glyph {
		t.Fatalf("cells must be written between autowrap off and on: %q", out)
	}
}

// A cursor-only frame writes no cells, so it has nothing to bracket.
func TestRenderDiffCursorOnlyFrameLeavesAutowrapAlone(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := r.RenderDiff(&buf, row(0, "a"), 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	buf.b = buf.b[:0]
	if _, err := r.RenderDiff(&buf, nil, 3, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	if out := string(buf.b); strings.Contains(out, seqAutowrapReset) || strings.Contains(out, seqAutowrapSet) {
		t.Fatalf("cursor-only frame toggled autowrap: %q", out)
	}
}

// A row holding a glyph whose width the terminal may measure differently is
// erased before it is repainted: after such a glyph the rest of the row can
// land shifted, and a shift to the left would keep the old frame's last
// columns on screen.
func TestRenderDiffErasesRowWithNonASCIIGlyph(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	dirty := append(row(0, "䷀q"), row(1, "plain")...)
	if _, err := r.RenderDiff(&buf, dirty, 0, 2, true, 0); err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	if !strings.Contains(out, "\x1b[1;1H"+seqSGRReset+seqEraseLineRight+"䷀") {
		t.Fatalf("row 0 was not erased from column 0 before its repaint: %q", out)
	}
	if strings.Count(out, seqEraseLineRight) != 1 {
		t.Fatalf("an ASCII-only row needs no erase, got %d erases: %q", strings.Count(out, seqEraseLineRight), out)
	}
}

// Only a full-row repaint may erase: a partial dirty list starting mid-row
// would wipe cells the caller did not resend.
func TestRenderDiffDoesNotEraseAPartialRow(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	dirty := []cell.DirtyCell{{X: 4, Y: 0, Cell: cell.Cell{Char: "✅", Width: 1}}}
	if _, err := r.RenderDiff(&buf, dirty, 0, 1, true, 0); err != nil {
		t.Fatal(err)
	}
	if out := string(buf.b); strings.Contains(out, seqEraseLineRight) {
		t.Fatalf("partial row erased: %q", out)
	}
}

func TestExitAltScreenRestoresAutowrap(t *testing.T) {
	if !strings.Contains(ExitAltScreenSeq(), seqAutowrapSet) {
		t.Fatalf("exit sequence must leave autowrap on for the shell: %q", ExitAltScreenSeq())
	}
}
