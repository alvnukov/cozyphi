package block

import (
	"slices"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// BashStatus mirrors bash tool status.
type BashStatus int

// Bash status values for a bash tool output row.
const (
	BashDone BashStatus = iota
	BashRunning
	BashError
	BashCancelled
	BashRejected
)

// BashBlock renders bash tool / "!cmd" output:
//
//	$ ls
//	  parser.go
//	  ...
//	  [Showing lines 10-100 of 100. Full/retained output: /tmp/cozyphi-bash-….log]
//
// Long output is truncated by the bash tool with a /tmp dump — this widget
// does not invent a useless "Show more" chrome.
type BashBlock struct {
	Command  string
	Output   string
	Status   BashStatus
	ExitCode int
	Expanded bool
	Theme    components.Theme

	// OnToggle is called when the user expands/collapses (click title / Enter).
	OnToggle func(expanded bool)

	titleH int // title row count; body clicks don't toggle (allow selection)
}

func (bashBlock *BashBlock) theme() components.Theme {
	if bashBlock.Theme.Success.Fg.Kind == 0 && bashBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return bashBlock.Theme
}

// Handle toggles expansion on Enter/space or a left-click on the title row.
func (bashBlock *BashBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			if bashBlock.hasBody() {
				bashBlock.toggle(ctx)
			}
		}
	case xui.MouseEvent:
		if e.Action != xui.MousePress || e.Button != xui.MouseLeft {
			return
		}
		// Only the title toggles expand; body stays selectable for copy-on-select.
		if bashBlock.hasBody() && e.Y >= 0 && e.Y < bashBlock.titleH {
			bashBlock.toggle(ctx)
		}
	}
}

// PointerShape offers the hand exactly where a click acts — the title row of
// a command with output — and a text beam over the rest.
func (bashBlock *BashBlock) PointerShape(_, y int) string {
	if bashBlock.hasBody() && y >= 0 && y < bashBlock.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// toggle flips expansion, notifies OnToggle, and schedules a redraw.
func (bashBlock *BashBlock) toggle(ctx *components.EventContext) {
	bashBlock.Expanded = !bashBlock.Expanded
	if bashBlock.OnToggle != nil {
		bashBlock.OnToggle(bashBlock.Expanded)
	}
	ctx.ConsumeAndRedraw()
}

// CopyText returns "$ command" plus output when present.
func (bashBlock *BashBlock) CopyText() string {
	var sb strings.Builder
	sb.WriteString("$ ")
	sb.WriteString(bashBlock.Command)
	out := strings.TrimRight(bashBlock.Output, "\n")
	if out != "" {
		sb.WriteByte('\n')
		sb.WriteString(out)
	}
	return sb.String()
}

func (bashBlock *BashBlock) hasBody() bool {
	return strings.TrimSpace(bashBlock.Output) != "" || (bashBlock.Status == BashError)
}

// Draw renders the "$ command" title and, when expanded, the truncated output body.
func (bashBlock *BashBlock) Draw(ctx components.DrawContext) components.Surface {
	th := bashBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	titleWrapped := components.WrapSpans(bashBlock.titleSpans(th), max(w-messageIndent, 1), ctx.Method)
	titleH := len(titleWrapped)
	bashBlock.titleH = titleH

	var bodyLines []components.RichLine
	backdrop := false
	if bashBlock.Expanded && bashBlock.hasBody() {
		bodyLines = bashBodyLines(bashBlock.Output, th, max(w-messageIndent-2, 1), ctx.Method)
		backdrop = true
	} else if bashBlock.Status == BashError {
		// A failed command shows its final output line — usually the actual
		// error — without the expand.
		if tail := lastOutputLine(bashBlock.Output); tail != "" {
			bodyLines = components.WrapSpans([]components.Span{
				{Text: tail, Style: th.Destructive},
			}, max(w-messageIndent-2, 1), ctx.Method)
		}
	}

	h := titleH + len(bodyLines)
	h = max(h, 1)
	s := components.NewSurface(w, h, bashBlock)
	y := 0
	for _, line := range titleWrapped {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range bodyLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	if backdrop {
		// The expanded output sits on a calm backdrop; the collapsed error
		// tail stays bare so the destructive line is the loudest thing there.
		components.FillRowsBg(&s, 2, titleH, titleH+len(bodyLines), th.BackgroundPanel)
	}
	gutter := quietGutter(th)
	if bashBlock.Status == BashError || bashBlock.Status == BashRejected {
		gutter = th.Destructive
	}
	components.HoverTitleRows(ctx, &s, bashBlock, bashBlock.titleH, th.BackgroundElement, bashBlock.hasBody())
	gutterBar(&s, gutter)
	return s
}

// titleSpans builds the "$ command [exit code] [arrow]" header spans.
func (bashBlock *BashBlock) titleSpans(th components.Theme) []components.Span {
	prefixStyle := th.Success
	switch bashBlock.Status {
	case BashError:
		prefixStyle = th.Destructive
	case BashRunning:
		prefixStyle = th.ToolName
	case BashCancelled, BashRejected:
		prefixStyle = th.Muted
	}

	cmdStyle := th.Foreground
	if bashBlock.Status == BashCancelled || bashBlock.Status == BashRejected {
		cmdStyle.Strikethrough = true
	}

	title := []components.Span{
		{Text: "$ ", Style: prefixStyle},
		{Text: bashBlock.Command, Style: cmdStyle},
	}
	switch bashBlock.Status {
	case BashCancelled:
		title = append(title, components.Span{Text: " (cancelled)", Style: th.Muted})
	case BashRejected:
		title = append(title, components.Span{Text: " (rejected)", Style: th.Muted})
	}
	if bashBlock.Status == BashDone && bashBlock.ExitCode != 0 {
		it := xui.Style{Italic: true}
		title = append(
			title,
			components.Span{Text: " (", Style: it},
			components.Span{Text: "exit code: ", Style: it},
			components.Span{
				Text:  strconv.Itoa(bashBlock.ExitCode),
				Style: xui.Style{Italic: true, Fg: th.Destructive.Fg},
			},
			components.Span{Text: ")", Style: it},
		)
	}
	if bashBlock.hasBody() {
		arrow := " ▶"
		if bashBlock.Expanded {
			arrow = " ▼"
		}
		title = append(title, components.Span{Text: arrow, Style: th.Muted})
	}
	return title
}

// lastOutputLine returns the last non-blank line of a command's output.
func lastOutputLine(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	for i := range slices.Backward(lines) {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

func bashBodyLines(output string, th components.Theme, width int, method xui.WidthMethod) []components.RichLine {
	if output == "" {
		return nil
	}
	text := strings.TrimRight(strings.ReplaceAll(output, "\r", ""), "\n")
	fg := th.Foreground
	fg.Dim = true
	spans := []components.Span{{Text: text + "\n", Style: fg}}
	if width < 1 {
		width = 1
	}
	return components.WrapSpans(spans, width, method)
}
