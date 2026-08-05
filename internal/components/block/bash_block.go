package block

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/xui"
)

// MaxBashPreviewLines matches the maximum number of preview lines
// (last N lines when truncated).
const MaxBashPreviewLines = 15

// BashStatus mirrors bash tool status.
type BashStatus int

const (
	BashDone BashStatus = iota
	BashRunning
	BashError
	BashCancelled
	BashRejected
)

// BashBlock renders bash tool output:
//
//	$ ls
//	  [... 14 lines truncated ...] Show more
//	  parser.go
//	  ...
type BashBlock struct {
	Command  string
	Output   string
	Status   BashStatus
	ExitCode int
	Expanded bool
	Theme    components.Theme

	// OnToggle is called when the user expands/collapses (click title / Enter).
	OnToggle func(expanded bool)
	// OnShowMore is called when "Show more" is activated.
	OnShowMore func(fullOutput string)

	showMoreHit hitRange // filled during Draw for mouse
	titleH      int      // title row count; body clicks don't toggle (allow selection)
}

type hitRange struct {
	valid     bool
	x0, x1, y int
}

func (bashBlock *BashBlock) theme() components.Theme {
	if bashBlock.Theme.Success.Fg.Kind == 0 && bashBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return bashBlock.Theme
}

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
		if bashBlock.showMoreHit.valid && e.Y == bashBlock.showMoreHit.y && e.X >= bashBlock.showMoreHit.x0 && e.X < bashBlock.showMoreHit.x1 {
			if bashBlock.OnShowMore != nil {
				bashBlock.OnShowMore(bashBlock.Output)
			} else {
				bashBlock.Expanded = true
			}
			ctx.ConsumeAndRedraw()
			return
		}
		// Only the title toggles expand; body stays selectable for copy-on-select.
		if bashBlock.hasBody() && e.Y >= 0 && e.Y < bashBlock.titleH {
			bashBlock.toggle(ctx)
		}
	}
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

func (bashBlock *BashBlock) Draw(ctx components.DrawContext) components.Surface {
	th := bashBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	bashBlock.showMoreHit = hitRange{}

	titleWrapped := components.WrapSpans(bashBlock.titleSpans(th), w, ctx.Method)
	titleH := len(titleWrapped)
	bashBlock.titleH = titleH

	var bodyLines []components.RichLine
	if bashBlock.Expanded && bashBlock.hasBody() {
		bodyLines = bashBodyLines(bashBlock.Output, true, th, w-2, ctx.Method, &bashBlock.showMoreHit)
		if bashBlock.showMoreHit.valid {
			bashBlock.showMoreHit.y += titleH
			bashBlock.showMoreHit.x0 += 2
			bashBlock.showMoreHit.x1 += 2
		}
	}

	h := titleH + len(bodyLines)
	if h < 1 {
		h = 1
	}
	s := components.NewSurface(w, h, bashBlock)
	y := 0
	for _, line := range titleWrapped {
		components.PaintSpans(&s, 0, y, line, ctx.Method)
		y++
	}
	for _, line := range bodyLines {
		components.PaintSpans(&s, 2, y, line, ctx.Method)
		y++
	}
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
		title = append(title,
			components.Span{Text: " (", Style: it},
			components.Span{Text: "exit code: ", Style: it},
			components.Span{Text: fmt.Sprintf("%d", bashBlock.ExitCode), Style: xui.Style{Italic: true, Fg: th.Destructive.Fg}},
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

func bashBodyLines(output string, showMore bool, th components.Theme, width int, method xui.WidthMethod, hit *hitRange) []components.RichLine {
	if output == "" {
		return nil
	}
	text := strings.TrimRight(strings.ReplaceAll(output, "\r", ""), "\n")
	lines := strings.Split(text, "\n")

	fg := th.Foreground
	fg.Dim = true

	var spans []components.Span
	tail := lines
	if len(lines) > MaxBashPreviewLines {
		n := len(lines) - MaxBashPreviewLines
		tail = lines[n:]
		trunc := fmt.Sprintf("[... %d lines truncated ...] ", n)
		spans = append(spans, components.Span{Text: trunc, Style: fg})
		if showMore {
			link := "Show more"
			if hit != nil {
				// x positions within the first painted body line (before left pad)
				hit.valid = true
				hit.x0 = xui.StringWidth(trunc, method)
				hit.x1 = hit.x0 + xui.StringWidth(link, method)
				hit.y = 0
			}
			spans = append(spans, components.Span{Text: link, Style: th.Accent})
		}
		spans = append(spans, components.Span{Text: "\n", Style: fg})
	}
	spans = append(spans, components.Span{Text: strings.Join(tail, "\n") + "\n", Style: fg})

	if width < 1 {
		width = 1
	}
	return components.WrapSpans(spans, width, method)
}
