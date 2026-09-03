package block

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

// DiffBlock renders a file-changing tool row (edit / write) as a diff card:
//
//	✓ edit pane.go +12 −3 ▼
//	  @@ -40,6 +40,7 @@
//	   old context
//	  +new line
//
// The title always carries the +N −M stats, so a collapsed card still says
// what the change weighed. An error is shown under the title even collapsed —
// a failed file change must never hide behind an expand.
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
	var errLines, hunkLines []components.RichLine
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
	if diffBlock.Expanded && diffBlock.hasBody() {
		hunkLines = diffBodyLines(diffBlock.Diff, th, bodyW, ctx.Method)
	}

	h := max(len(titleLines)+len(errLines)+len(hunkLines), 1)
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
	hunkStart := y
	for _, line := range hunkLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	// The hunks sit on a calm backdrop; error rows stay bare so the
	// destructive text is the loudest thing on the row.
	components.FillRowsBg(&s, 2, hunkStart, hunkStart+len(hunkLines), th.BackgroundPanel)
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

// diffBodyLines colors the hunks by unified-diff prefix. The ---/+++ file
// header duplicates the path the title already names, so it is dropped.
func diffBodyLines(diff string, th components.Theme, width int, method xui.WidthMethod) []components.RichLine {
	var out []components.RichLine
	for line := range strings.Lines(strings.TrimRight(diff, "\n")) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		st := th.Foreground
		switch {
		case strings.HasPrefix(line, "+"):
			st = th.Success
		case strings.HasPrefix(line, "-"):
			st = th.Destructive
		case strings.HasPrefix(line, "@@"):
			st = th.Secondary
		default:
			st.Dim = true
		}
		out = append(out, components.WrapSpans(
			[]components.Span{{Text: line, Style: st}}, width, method,
		)...)
	}
	return out
}
