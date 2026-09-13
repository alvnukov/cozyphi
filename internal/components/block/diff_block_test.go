package block_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

const sampleDiff = "--- a/pane.go\n" +
	"+++ b/pane.go\n" +
	"@@ -1,3 +1,4 @@\n" +
	" kept line\n" +
	"-removed line\n" +
	"+added line\n" +
	"+second added\n"

func drawDiff(d *block.DiffBlock) string {
	s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}})
	return components.SurfaceText(s)
}

func TestDiffStatsCountsHunkLinesOnly(t *testing.T) {
	added, removed := block.DiffStats(sampleDiff)
	if added != 2 || removed != 1 {
		t.Fatalf("stats: got +%d −%d, want +2 −1", added, removed)
	}
}

func TestDiffBlockTitleCarriesStatsWhenCollapsed(t *testing.T) {
	d := &block.DiffBlock{
		Name:   "edit",
		Path:   "pane.go",
		Diff:   sampleDiff,
		Status: status.ToolDone,
		Theme:  components.DefaultTheme(),
	}
	txt := drawDiff(d)
	for _, want := range []string{"edit", "pane.go", "+2", "−1", "▶"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("collapsed title missing %q: %q", want, txt)
		}
	}
	if strings.Contains(txt, "added line") {
		t.Fatalf("collapsed card must not show hunks: %q", txt)
	}
}

func TestDiffBlockExpandedShowsHunksWithoutFileHeader(t *testing.T) {
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     sampleDiff,
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	txt := drawDiff(d)
	for _, want := range []string{"1   kept line", "2 - removed line", "2 + added line", "▼"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("expanded body missing %q: %q", want, txt)
		}
	}
	for _, unwanted := range []string{"--- a/", "+++ b/", "@@"} {
		if strings.Contains(txt, unwanted) {
			t.Fatalf("patch chrome %q belongs to CopyText, not the card: %q", unwanted, txt)
		}
	}
}

// bodyRows returns the drawn card's rows below the title, trimmed right.
func bodyRows(t *testing.T, d *block.DiffBlock, width int) []string {
	t.Helper()
	s := d.Draw(components.DrawContext{
		Max:    components.Size{Width: width, Height: 40},
		Method: xui.WidthUnicode,
	})
	var rows []string
	for line := range strings.SplitSeq(strings.TrimRight(components.SurfaceText(s), "\n"), "\n") {
		rows = append(rows, strings.TrimRight(line, " "))
	}
	return rows[1:]
}

const twoHunkDiff = "--- a/pane.go\n" +
	"+++ b/pane.go\n" +
	"@@ -8,3 +8,3 @@ func draw() {\n" +
	" const keep = 1\n" +
	"-\tvar old string\n" +
	"+\tvar renamed string\n" +
	"@@ -98,2 +98,3 @@\n" +
	" func other() {\n" +
	"+\treturn errors.New(\"boom\")\n"

// TestDiffBlockColumnsAndNumbering pins the body layout against Claude Code's:
// a right-aligned line number as wide as the largest one in the diff, the
// marker, then the code. Context and added rows count in the new file, removed
// rows in the old one, and a jump between hunks is one thin rule.
func TestDiffBlockColumnsAndNumbering(t *testing.T) {
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     twoHunkDiff,
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	want := []string{
		"▏     8   const keep = 1",
		"▏     9 -     var old string",
		"▏     9 +     var renamed string",
		"▏         ···",
		"▏    98   func other() {",
		"▏    99 +     return errors.New(\"boom\")",
	}
	got := bodyRows(t, d, 60)
	if len(got) != len(want) {
		t.Fatalf("body rows: got %d %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// TestDiffBlockRowBackgroundsByKind: the row tint is the change, so it runs
// the full width — added green, removed red, context and rule on the panel.
func TestDiffBlockRowBackgroundsByKind(t *testing.T) {
	th := components.DefaultTheme()
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     twoHunkDiff,
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    th,
	}
	s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}, Method: xui.WidthUnicode})
	want := []xui.Color{
		th.BackgroundPanel.Bg,
		th.DiffRemovedBg.Bg,
		th.DiffAddedBg.Bg,
		th.BackgroundPanel.Bg,
		th.BackgroundPanel.Bg,
		th.DiffAddedBg.Bg,
	}
	for i, bg := range want {
		y := i + 1 // row 0 is the title
		for x := 2; x < s.Size.Width; x++ {
			if got := s.Buffer[y*s.Size.Width+x].Style.Bg; got != bg {
				t.Fatalf("row %d column %d: background %v, want %v", i, x, got, bg)
			}
		}
	}
}

// TestDiffBlockMarkerTakesSemanticColor: the +/- marker carries the
// palette's diff roles while the number column stays muted chrome. The
// opencode row proves the inheritance path, Light (VS) the explicit pair.
func TestDiffBlockMarkerTakesSemanticColor(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   components.Theme
	}{
		{"opencode", components.DefaultTheme()},
		{"Light (VS)", components.VSLightTheme()},
	} {
		d := &block.DiffBlock{
			Name:     "edit",
			Path:     "pane.go",
			Diff:     twoHunkDiff,
			Status:   status.ToolDone,
			Expanded: true,
			Theme:    tc.th,
		}
		s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}, Method: xui.WidthUnicode})
		lines := strings.Split(components.SurfaceText(s), "\n")
		for _, row := range []struct {
			y     int
			glyph string
			want  xui.Style
		}{
			{2, "-", tc.th.DiffRemove},
			{3, "+", tc.th.DiffAdd},
		} {
			b := strings.Index(lines[row.y], row.glyph)
			if b < 0 {
				t.Fatalf("%s: row %d has no %q marker: %q", tc.name, row.y, row.glyph, lines[row.y])
			}
			// SurfaceText offsets are runes (the gutter glyph is 3 bytes), the
			// buffer is cells: convert before indexing.
			x := utf8.RuneCountInString(lines[row.y][:b])
			if got := s.Buffer[row.y*s.Size.Width+x].Style.Fg; got != row.want.Fg {
				t.Fatalf("%s: row %d marker color got %v, want %v", tc.name, row.y, got, row.want.Fg)
			}
		}
		numB := strings.IndexAny(lines[2], "0123456789")
		numX := utf8.RuneCountInString(lines[2][:numB])
		if got := s.Buffer[2*s.Size.Width+numX].Style.Fg; got != tc.th.Muted.Fg {
			t.Fatalf("%s: the number column stays muted chrome, got %v", tc.name, got)
		}
	}
}

// TestDiffBlockSelectionCopiesCodeOnly: numbers, markers and the rule are
// chrome, so a drag over the body yields the file's own text — indentation
// kept, nothing the card drew around it.
func TestDiffBlockSelectionCopiesCodeOnly(t *testing.T) {
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     twoHunkDiff,
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}, Method: xui.WidthUnicode})
	got := components.ExtractSurfaceText(s, 0, 1, s.Size.Width-1, s.Size.Height-1)
	want := "const keep = 1\n" +
		"    var old string\n" +
		"    var renamed string\n" +
		"\n" +
		"func other() {\n" +
		"    return errors.New(\"boom\")"
	if got != want {
		t.Fatalf("selection copy:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(d.CopyText(), "@@ -8,3 +8,3 @@") {
		t.Fatalf("CopyText is the deliberate patch copy: %q", d.CopyText())
	}
}

// TestDiffBlockClipsLongRows: a diff is read down its left edge, so an
// over-long row is cut with a muted "…" that the clipboard never sees.
func TestDiffBlockClipsLongRows(t *testing.T) {
	long := strings.Repeat("x", 200)
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     "@@ -1,1 +1,2 @@\n kept\n+" + long + "\n",
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	rows := bodyRows(t, d, 30)
	if len(rows) != 2 {
		t.Fatalf("a clipped row must not wrap: got %d rows %q", len(rows), rows)
	}
	if !strings.HasSuffix(rows[1], "…") {
		t.Fatalf("clipped row must end in …: %q", rows[1])
	}
	if n := len([]rune(rows[1])); n != 30 {
		t.Fatalf("clipped row spans %d columns, want the full 30: %q", n, rows[1])
	}
}

// TestDiffBlockHighlightsByFileName: chroma is picked by the changed file's
// extension, and prose no lexer claims leaves the code in plain foreground
// instead of failing the draw.
func TestDiffBlockHighlightsByFileName(t *testing.T) {
	th := components.DefaultTheme()
	styleOf := func(path, body, needle string) xui.Style {
		d := &block.DiffBlock{
			Name: "edit", Path: path, Diff: "@@ -1,1 +1,2 @@\n keep\n+" + body + "\n",
			Status: status.ToolDone, Expanded: true, Theme: th,
		}
		s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 10}, Method: xui.WidthUnicode})
		row := 2 // title, context, added
		text := strings.Split(components.SurfaceText(s), "\n")[row]
		idx := strings.Index(text, needle)
		if idx < 0 {
			t.Fatalf("%s: row %q has no %q", path, text, needle)
		}
		return s.Buffer[row*s.Size.Width+idx].Style
	}
	if got := styleOf("pane.go", "func main() {}", "func"); got.Fg != th.Syntax.Keyword.Fg {
		t.Fatalf("go keyword: got %v, want %v", got.Fg, th.Syntax.Keyword.Fg)
	}
	if got := styleOf("notes.unknownext", "plain prose here", "prose"); got.Fg != th.Foreground.Fg {
		t.Fatalf("unclaimed text must stay plain: got %v", got.Fg)
	}
}

func TestDiffBlockErrorVisibleCollapsed(t *testing.T) {
	d := &block.DiffBlock{
		Name:   "edit",
		Path:   "pane.go",
		Error:  "hash mismatch: the file changed on disk\nsecond error line",
		Status: status.ToolError,
		Theme:  components.DefaultTheme(),
	}
	txt := drawDiff(d)
	if !strings.Contains(txt, "Error: hash mismatch") {
		t.Fatalf("collapsed card must show the failure: %q", txt)
	}
	if strings.Contains(txt, "second error line") {
		t.Fatalf("collapsed card shows only the first error line: %q", txt)
	}
}

func TestToolBlockErrorVisibleCollapsed(t *testing.T) {
	b := &block.ToolBlock{
		Name:   "grep",
		Detail: `"pat" — 0 matches`,
		Error:  "ripgrep exited with code 2\ndetails follow",
		Status: status.ToolError,
		Theme:  components.DefaultTheme(),
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "Error: ripgrep exited with code 2") {
		t.Fatalf("collapsed tool row must show the failure: %q", txt)
	}
	if strings.Contains(txt, "details follow") {
		t.Fatalf("collapsed tool row shows only the first error line: %q", txt)
	}
}

func TestBashBlockErrorShowsFinalLineCollapsed(t *testing.T) {
	b := &block.BashBlock{
		Command: "make test",
		Output:  "long build log\nFAIL: TestX broke\n",
		Status:  block.BashError,
		Theme:   components.DefaultTheme(),
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "FAIL: TestX broke") {
		t.Fatalf("collapsed failed command must show its last line: %q", txt)
	}
	if strings.Contains(txt, "long build log") {
		t.Fatalf("collapsed failed command shows only the tail: %q", txt)
	}
}
