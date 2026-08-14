package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
)

// ThinkingBlock renders reasoning: collapsible header with spinner
// while streaming, ✓ when done, and dim italic body when expanded.
type ThinkingBlock struct {
	Text        string
	Streaming   bool
	Interrupted bool
	Expanded    bool
	Theme       components.Theme
	Spinner     *status.Spinner
	OnToggle    func(expanded bool)

	titleH int
}

func (t *ThinkingBlock) theme() components.Theme {
	if t.Theme.Success.Fg.Kind == 0 && t.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return t.Theme
}

// Handle toggles expansion on Enter/space or a left-click on the title row.
func (t *ThinkingBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			t.Expanded = !t.Expanded
			if t.OnToggle != nil {
				t.OnToggle(t.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < t.titleH {
			t.Expanded = !t.Expanded
			if t.OnToggle != nil {
				t.OnToggle(t.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	}
}

// CopyText returns thinking body text.
func (t *ThinkingBlock) CopyText() string { return t.Text }

// Draw renders the "Thinking" header with spinner/done icon and the
// dim italic reasoning body when expanded.
func (t *ThinkingBlock) Draw(ctx components.DrawContext) components.Surface {
	th := t.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	icon := "✓"
	iconSt := th.Success
	labelSt := th.Muted
	if t.Streaming {
		icon = "..."
		iconSt = th.ToolName
		if t.Spinner != nil {
			icon = t.Spinner.Glyph()
		}
		labelSt = th.ToolName
	}
	if t.Interrupted {
		icon = "⊘"
		iconSt = th.Warning
		labelSt = th.Warning
	}

	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
		{Text: "Thinking", Style: labelSt},
	}
	if t.Interrupted {
		spans = append(spans, components.Span{Text: " (interrupted)", Style: th.Warning})
	}
	arrow := " ▶"
	if t.Expanded {
		arrow = " ▼"
	}
	spans = append(spans, components.Span{Text: arrow, Style: th.Muted})

	titleLines := components.WrapSpans(spans, w, ctx.Method)
	t.titleH = len(titleLines)

	var bodyLines []components.RichLine
	if t.Expanded && strings.TrimSpace(t.Text) != "" {
		body := th.Muted
		body.Italic = true
		body.Dim = true
		bodyLines = components.WrapSpans([]components.Span{{Text: t.Text, Style: body}}, w, ctx.Method)
	}

	h := len(titleLines) + len(bodyLines)
	h = max(h, 1)
	s := components.NewSurface(w, h, t)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, 0, y, line, ctx.Method)
		y++
	}
	for _, line := range bodyLines {
		components.PaintSpans(&s, 0, y, line, ctx.Method)
		y++
	}
	return s
}
