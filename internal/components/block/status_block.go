package block

import (
	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

// StatusBlock renders activity chrome: "✓ Thinking ▸".
type StatusBlock struct {
	Label      string
	Done       bool
	Expandable bool
	Expanded   bool
	Theme      components.Theme
	OnToggle   func(expanded bool)
}

func (statusBlock *StatusBlock) theme() components.Theme {
	if statusBlock.Theme.Success.Fg.Kind == 0 && statusBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return statusBlock.Theme
}

// Handle toggles expansion on Enter/space or mouse press when Expandable.
func (statusBlock *StatusBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if !statusBlock.Expandable {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			statusBlock.Expanded = !statusBlock.Expanded
			if statusBlock.OnToggle != nil {
				statusBlock.OnToggle(statusBlock.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft {
			statusBlock.Expanded = !statusBlock.Expanded
			if statusBlock.OnToggle != nil {
				statusBlock.OnToggle(statusBlock.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	}
}

// Draw renders the "✓/⋯ Label ▶" activity line.
func (statusBlock *StatusBlock) Draw(ctx components.DrawContext) components.Surface {
	th := statusBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	icon := "✓"
	iconStyle := th.Success
	if !statusBlock.Done {
		icon = "⋯"
		iconStyle = th.ToolName
	}
	spans := []components.Span{
		{Text: icon + " ", Style: iconStyle},
		{Text: statusBlock.Label, Style: th.Muted},
	}
	if statusBlock.Expandable {
		arrow := " ▶"
		if statusBlock.Expanded {
			arrow = " ▼"
		}
		spans = append(spans, components.Span{Text: arrow, Style: th.Muted})
	}
	lines := components.WrapSpans(spans, w, ctx.Method)
	return components.PaintRichLines(w, lines, ctx.Method, statusBlock)
}
