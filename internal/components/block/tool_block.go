package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

// ToolBlock renders a generic tool_use row: status glyph, name,
// detail, and optional expandable output body.
type ToolBlock struct {
	Name     string
	Detail   string
	Output   string
	Error    string
	Status   status.ToolStatus
	Expanded bool
	Theme    components.Theme
	Spinner  *status.Spinner
	OnToggle func(expanded bool)

	titleH int
}

func (toolBlock *ToolBlock) theme() components.Theme {
	if toolBlock.Theme.Success.Fg.Kind == 0 && toolBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return toolBlock.Theme
}

// HasBody reports whether the row has anything to unfold: output or an
// error. A row without one has no expand arrow and ignores toggles.
func (toolBlock *ToolBlock) HasBody() bool {
	return strings.TrimSpace(toolBlock.Output) != "" || strings.TrimSpace(toolBlock.Error) != ""
}

// Handle toggles expansion on Enter/space or a left-click on the title row.
func (toolBlock *ToolBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if !toolBlock.HasBody() {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			toolBlock.Expanded = !toolBlock.Expanded
			if toolBlock.OnToggle != nil {
				toolBlock.OnToggle(toolBlock.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < toolBlock.titleH {
			toolBlock.Expanded = !toolBlock.Expanded
			if toolBlock.OnToggle != nil {
				toolBlock.OnToggle(toolBlock.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	}
}

// PointerShape offers the hand exactly where a click acts — the title row of
// a block with a body — and a text beam over the rest (selectable output).
func (toolBlock *ToolBlock) PointerShape(_, y int) string {
	if toolBlock.HasBody() && y >= 0 && y < toolBlock.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// CopyText returns name, detail, and body.
func (toolBlock *ToolBlock) CopyText() string {
	var b strings.Builder
	b.WriteString(toolBlock.Name)
	if toolBlock.Detail != "" {
		b.WriteByte(' ')
		b.WriteString(toolBlock.Detail)
	}
	if out := strings.TrimSpace(toolBlock.Output); out != "" {
		b.WriteByte('\n')
		b.WriteString(out)
	}
	if err := strings.TrimSpace(toolBlock.Error); err != "" {
		b.WriteByte('\n')
		b.WriteString("Error: ")
		b.WriteString(err)
	}
	return b.String()
}

// Draw renders the tool status glyph, name, detail, and optional output body.
func (toolBlock *ToolBlock) Draw(ctx components.DrawContext) components.Surface {
	th := toolBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	icon := "✓"
	iconSt := th.Success
	switch toolBlock.Status {
	case status.ToolRunning, status.ToolQueued:
		icon = "..."
		iconSt = th.ToolName
		if toolBlock.Spinner != nil {
			icon = toolBlock.Spinner.Glyph()
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
	case status.ToolLive:
		// The footer's watch glyph, breathing on the same wall clock, so
		// the row and the indicator pulse in one rhythm.
		icon = "⏱"
		iconSt = components.PulseStyle(th.ToolName, th.Muted)
	}

	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
		{Text: toolBlock.Name, Style: th.Foreground},
	}
	if toolBlock.Detail != "" {
		spans = append(spans, components.Span{Text: " " + toolBlock.Detail, Style: th.Muted})
	}
	switch toolBlock.Status {
	case status.ToolCancelled:
		spans = append(spans, components.Span{Text: " (cancelled)", Style: th.Muted})
	case status.ToolRejected:
		spans = append(spans, components.Span{Text: " (rejected)", Style: th.Muted})
	}
	if toolBlock.HasBody() {
		arrow := " ▶"
		if toolBlock.Expanded {
			arrow = " ▼"
		}
		spans = append(spans, components.Span{Text: arrow, Style: th.Muted})
	}

	titleLines := components.WrapSpans(spans, max(w-messageIndent, 1), ctx.Method)
	toolBlock.titleH = len(titleLines)

	bodyW := w
	if bodyW > 2 {
		bodyW -= 2
	}
	bodyW = max(bodyW-messageIndent, 1)
	var errLines, outLines []components.RichLine
	if err := strings.TrimSpace(toolBlock.Error); err != "" {
		// The failure never hides behind the expand: a collapsed row shows
		// the first error line, expanding reveals the rest.
		if !toolBlock.Expanded {
			err, _, _ = strings.Cut(err, "\n")
		}
		errLines = components.WrapSpans([]components.Span{
			{Text: "Error: " + err, Style: th.Destructive},
		}, bodyW, ctx.Method)
	}
	if toolBlock.Expanded {
		if out := strings.TrimSpace(toolBlock.Output); out != "" {
			fg := th.Foreground
			fg.Dim = true
			outLines = components.WrapSpans([]components.Span{
				{Text: out, Style: fg},
			}, bodyW, ctx.Method)
		}
	}

	h := len(titleLines) + len(errLines) + len(outLines)
	h = max(h, 1)
	s := components.NewSurface(w, h, toolBlock)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range errLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	outStart := y
	for _, line := range outLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	// The expanded output sits on a calm backdrop; error rows stay bare so
	// the destructive text is the loudest thing on the row.
	components.FillRowsBg(&s, 2, outStart, outStart+len(outLines), th.BackgroundPanel)
	if components.Hovering(ctx, toolBlock) && toolBlock.HasBody() {
		components.ApplyHoverRows(&s, 0, toolBlock.titleH, th.BackgroundElement)
	}
	gutter := quietGutter(th)
	if toolBlock.Status == status.ToolError || toolBlock.Status == status.ToolRejected {
		gutter = th.Destructive
	}
	gutterBar(&s, gutter)
	return s
}
