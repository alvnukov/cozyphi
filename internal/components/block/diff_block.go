package block

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// DiffBlock renders a file-changing tool row (edit / write) as a diff card:
//
//	✓ edit pane.go +12 −3 ▼
//	   40   old context
//	   41 - gone
//	   41 + new line
//
// The body reads like an editor: the file's own line numbers, the change
// marker, then the code under chroma highlighting, each row on a tint that
// says what happened to it. The title always carries the +N −M stats, so a
// collapsed card still says what the change weighed. An error is shown under
// the title even collapsed — a failed file change must never hide behind an
// expand.
type DiffBlock struct {
	Name     string // tool name shown on the row (edit, write)
	Path     string // display path of the changed file
	Diff     string // unified diff body; empty until the run completes
	Error    string
	Status   status.ToolStatus
	Expanded bool
	Theme    components.Theme
	Spinner  *status.Spinner
	OnToggle func(expanded bool)

	titleH int
}

func (diffBlock *DiffBlock) theme() components.Theme {
	if diffBlock.Theme.Success.Fg.Kind == 0 && diffBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return diffBlock.Theme
}

func (diffBlock *DiffBlock) hasBody() bool {
	return strings.TrimSpace(diffBlock.Diff) != ""
}

// Handle toggles expansion on Enter/space or a left-click on the title row.
func (diffBlock *DiffBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if !diffBlock.hasBody() {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			diffBlock.toggle(ctx)
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < diffBlock.titleH {
			diffBlock.toggle(ctx)
		}
	}
}

func (diffBlock *DiffBlock) toggle(ctx *components.EventContext) {
	diffBlock.Expanded = !diffBlock.Expanded
	if diffBlock.OnToggle != nil {
		diffBlock.OnToggle(diffBlock.Expanded)
	}
	ctx.ConsumeAndRedraw()
}

// PointerShape offers the hand on the toggling title row, text elsewhere.
func (diffBlock *DiffBlock) PointerShape(_, y int) string {
	if diffBlock.hasBody() && y >= 0 && y < diffBlock.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// CopyText returns the row header and the full diff.
func (diffBlock *DiffBlock) CopyText() string {
	var b strings.Builder
	b.WriteString(diffBlock.Name)
	if diffBlock.Path != "" {
		b.WriteByte(' ')
		b.WriteString(diffBlock.Path)
	}
	if err := strings.TrimSpace(diffBlock.Error); err != "" {
		b.WriteString("\nError: ")
		b.WriteString(err)
	}
	if diff := strings.TrimSpace(diffBlock.Diff); diff != "" {
		b.WriteByte('\n')
		b.WriteString(diff)
	}
	return b.String()
}

// DiffStats counts added and removed lines in a unified diff, ignoring the
// ---/+++ file header lines.
func DiffStats(diff string) (added, removed int) {
	for line := range strings.Lines(diff) {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

// Draw renders the stats title and, when expanded, the colored diff body.
func (diffBlock *DiffBlock) Draw(ctx components.DrawContext) components.Surface {
	th := diffBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	titleLines := components.WrapSpans(diffBlock.titleSpans(th), max(w-messageIndent, 1), ctx.Method)
	diffBlock.titleH = len(titleLines)

	bodyW := max(w-messageIndent-2, 1)
	var errLines []components.RichLine
	if err := strings.TrimSpace(diffBlock.Error); err != "" {
		// The failure is visible collapsed; the expand reveals the rest.
		errText := err
		if !diffBlock.Expanded {
			errText, _, _ = strings.Cut(errText, "\n")
		}
		errLines = components.WrapSpans([]components.Span{
			{Text: "Error: " + errText, Style: th.Destructive},
		}, bodyW, ctx.Method)
	}
	var rows []diffRow
	if diffBlock.Expanded && diffBlock.hasBody() {
		rows = parseDiffRows(diffBlock.Diff)
	}

	h := max(len(titleLines)+len(errLines)+len(rows), 1)
	s := components.NewSurface(w, h, diffBlock)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range errLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	// The body carries its own per-row backdrop; error rows stay bare so the
	// destructive text is the loudest thing on the row.
	diffBlock.paintBody(&s, y, rows, th, ctx.Method)
	gutter := quietGutter(th)
	if diffBlock.Status == status.ToolError || diffBlock.Status == status.ToolRejected {
		gutter = th.Destructive
	}
	gutterBar(&s, gutter)
	return s
}

// titleSpans builds "glyph name path +N −M [state] [arrow]".
func (diffBlock *DiffBlock) titleSpans(th components.Theme) []components.Span {
	icon := "✓"
	iconSt := th.Success
	switch diffBlock.Status {
	case status.ToolRunning, status.ToolQueued:
		icon = "..."
		iconSt = th.ToolName
		if diffBlock.Spinner != nil {
			icon = diffBlock.Spinner.Glyph()
		}
	case status.ToolError:
		icon = "✗"
		iconSt = th.Destructive
	case status.ToolCancelled:
		icon = "⊘"
		iconSt = th.Muted
	case status.ToolRejected:
		icon = "⊘"
		iconSt = th.Destructive
	}

	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
		{Text: diffBlock.Name, Style: th.Foreground},
	}
	if diffBlock.Path != "" {
		spans = append(spans, components.Span{Text: " " + diffBlock.Path, Style: th.Foreground})
	}
	if added, removed := DiffStats(diffBlock.Diff); added > 0 || removed > 0 {
		spans = append(spans, components.Span{
			Text:  fmt.Sprintf(" +%d", added),
			Style: th.Success,
		}, components.Span{
			Text:  fmt.Sprintf(" −%d", removed),
			Style: th.Destructive,
		})
	}
	switch diffBlock.Status {
	case status.ToolCancelled:
		spans = append(spans, components.Span{Text: " (cancelled)", Style: th.Muted})
	case status.ToolRejected:
		spans = append(spans, components.Span{Text: " (rejected)", Style: th.Muted})
	}
	if diffBlock.hasBody() {
		arrow := " ▶"
		if diffBlock.Expanded {
			arrow = " ▼"
		}
		spans = append(spans, components.Span{Text: arrow, Style: th.Muted})
	}
	return spans
}

// diffRowKind is what a body row says about its line: kept, added, removed,
// or the thin rule where the file skips from one hunk to the next.
type diffRowKind int

const (
	diffContext diffRowKind = iota
	diffAdded
	diffRemoved
	diffGap
)

// diffRow is one rendered body line: the file line it carries (0 for a gap),
// and the code with the unified-diff marker already stripped off.
type diffRow struct {
	kind diffRowKind
	num  int
	code string
}

// gapGlyphs is the hunk separator — a rule, not a line of the file, so it
// carries no number and copies as nothing.
const gapGlyphs = "···"

// parseDiffRows turns a unified diff into numbered body rows. The ---/+++ file
// header duplicates the path the title already names and is dropped; every @@
// header reseeds the counters and, from the second hunk on, leaves a gap row
// where the file jumps ahead. Context and added rows are numbered in the new
// file, removed rows in the old one — the two columns a reader compares.
func parseDiffRows(diff string) []diffRow {
	var rows []diffRow
	// A diff with no @@ header at all (a whole-file write) still numbers from
	// the top rather than from nowhere.
	oldNum, newNum, seenHunk := 1, 1, false
	for line := range strings.Lines(strings.TrimRight(diff, "\n")) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case !seenHunk && (strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---")):
			// The file header stands before the first hunk. Inside one, the
			// same prefix is a line of the file that happens to start with a
			// dash — dropping it would lose content.
		case strings.HasPrefix(line, "@@"):
			o, n, ok := parseHunkHeader(line)
			if !ok {
				continue
			}
			if seenHunk {
				rows = append(rows, diffRow{kind: diffGap})
			}
			oldNum, newNum, seenHunk = o, n, true
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" annotates the patch, not the file.
		case strings.HasPrefix(line, "+"):
			rows = append(rows, diffRow{kind: diffAdded, num: newNum, code: line[1:]})
			newNum++
		case strings.HasPrefix(line, "-"):
			rows = append(rows, diffRow{kind: diffRemoved, num: oldNum, code: line[1:]})
			oldNum++
		default:
			rows = append(rows, diffRow{kind: diffContext, num: newNum, code: strings.TrimPrefix(line, " ")})
			oldNum++
			newNum++
		}
	}
	return rows
}

// parseHunkHeader reads the old and new starting line from "@@ -a,b +c,d @@".
func parseHunkHeader(line string) (oldStart, newStart int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0, false
	}
	start := func(field, sign string) (int, bool) {
		if !strings.HasPrefix(field, sign) {
			return 0, false
		}
		num, _, _ := strings.Cut(field[1:], ",")
		n, err := strconv.Atoi(num)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	oldStart, okOld := start(fields[1], "-")
	newStart, okNew := start(fields[2], "+")
	return oldStart, newStart, okOld && okNew
}

// paintBody draws the rows at y and down: a right-aligned line number, the
// +/-/space marker, then the code, each row on a full-width tint that says
// what happened to it. Number and marker columns are chrome — a drag over the
// body selects and copies the code alone, the way the file reads on disk.
func (diffBlock *DiffBlock) paintBody(
	s *components.Surface,
	y int,
	rows []diffRow,
	th components.Theme,
	method xui.WidthMethod,
) {
	if len(rows) == 0 {
		return
	}
	numW := 1
	for _, row := range rows {
		numW = max(numW, len(strconv.Itoa(row.num)))
	}
	// [num][ ][marker][ ][code]
	codeX := messageIndent + 2 + numW + 3
	codeW := max(s.Size.Width-codeX, 1)
	code := diffBlock.codeLines(rows, th)

	for i, row := range rows {
		rowY := y + i
		bg := th.BackgroundPanel
		switch row.kind {
		case diffAdded:
			bg = th.DiffAddedBg
		case diffRemoved:
			bg = th.DiffRemovedBg
		case diffContext, diffGap:
		}
		if row.kind == diffGap {
			components.PaintSpans(s, codeX, rowY, components.RichLine{
				{Text: gapGlyphs, Style: th.Muted},
			}, method)
		} else {
			marker := " "
			switch row.kind {
			case diffAdded:
				marker = "+"
			case diffRemoved:
				marker = "-"
			case diffContext, diffGap:
			}
			components.PaintSpans(s, messageIndent+2, rowY, components.RichLine{
				{Text: fmt.Sprintf("%*d %s ", numW, row.num, marker), Style: th.Muted},
			}, method)
			clipped, cut := clipSpans(code[i], codeW, th, method)
			end := codeX + components.PaintSpans(s, codeX, rowY, clipped, method)
			if cut {
				// The "…" says the row goes on; it is not part of the file.
				components.MarkChrome(s, end-1, rowY, end)
			}
		}
		components.FillRowsBg(s, 2, rowY, rowY+1, bg)
		// The whole rule is chrome; a code row keeps only its left columns.
		markTo := codeX
		if row.kind == diffGap {
			markTo = s.Size.Width
		}
		components.MarkChrome(s, 0, rowY, markTo)
	}
}

// codeLines highlights the body's code as one document, so a string or
// comment that spans lines is colored across them. Old and new lines share
// the stream, which is what a reader sees anyway; a lexer that does not line
// up row for row is dropped for the plain style rather than mis-numbered.
func (diffBlock *DiffBlock) codeLines(rows []diffRow, th components.Theme) []components.RichLine {
	plain := th.Foreground
	raw := make([]string, len(rows))
	for i, row := range rows {
		// Tabs are invisible geometry in a cell grid; spaces are honest.
		raw[i] = strings.ReplaceAll(row.code, "\t", "    ")
	}
	out := make([]components.RichLine, len(rows))
	lines := text.HighlightCodeLines(strings.Join(raw, "\n"), diffBlock.lexerName(), th, plain)
	if len(lines) != len(rows) {
		for i, line := range raw {
			out[i] = components.RichLine{{Text: line, Style: plain}}
		}
		return out
	}
	for i, line := range lines {
		out[i] = components.RichLine(line)
	}
	return out
}

// lexerName is what chroma is asked to highlight the body as: the changed
// file's name, which carries the extension (chroma matches on the base name,
// so a path is as good as a file name). The block's own path wins; a diff
// pasted without one falls back to the +++ header. An unknown name simply
// leaves the body in plain foreground.
func (diffBlock *DiffBlock) lexerName() string {
	if path := strings.TrimSpace(diffBlock.Path); path != "" {
		return path
	}
	for line := range strings.Lines(diffBlock.Diff) {
		if !strings.HasPrefix(line, "+++") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "+++"))
		name, _, _ = strings.Cut(name, "\t")
		name = strings.TrimPrefix(strings.TrimSpace(name), "b/")
		if name != "" && name != "/dev/null" {
			return name
		}
	}
	return ""
}

// clipSpans cuts spans to width columns, marking the cut with a muted "…".
// A diff row is read from its left edge, so an over-long row is clipped, never
// wrapped: wrapping would put code under the number column and break the
// vertical read of the change.
func clipSpans(
	line components.RichLine,
	width int,
	th components.Theme,
	method xui.WidthMethod,
) (components.RichLine, bool) {
	if components.MeasureSpans(line, method) <= width {
		return line, false
	}
	room := max(width-1, 0)
	out := make(components.RichLine, 0, len(line)+1)
	col := 0
	for _, sp := range line {
		var kept strings.Builder
		rest := sp.Text
		for rest != "" {
			cluster, cw, next := xui.FirstGrapheme(rest, method)
			cw = max(cw, 1)
			if col+cw > room {
				break
			}
			kept.WriteString(cluster)
			col += cw
			rest = next
		}
		if kept.Len() > 0 {
			out = append(out, components.Span{Text: kept.String(), Style: sp.Style})
		}
		if col >= room {
			break
		}
	}
	return append(out, components.Span{Text: "…", Style: th.Muted}), true
}
