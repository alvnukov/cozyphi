package block

import (
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// ThinkingBlock renders reasoning: a collapsed one-line header with spinner
// while streaming, "Thought for <span>" when done, expandable on demand to
// the themed Markdown body.
type ThinkingBlock struct {
	Text      string
	Streaming bool
	// Model names the engine behind a live stream: the streaming header
	// leads with it ("<model> · thinking") so the transcript says who is
	// talking while the footer stays quiet.
	Model       string
	Interrupted bool
	// Duration is the wall-clock span of the reasoning once finished; the
	// header appends it opencode-style when it is at least a second.
	Duration time.Duration
	Expanded bool
	Theme    components.Theme
	Spinner  *status.Spinner
	OnToggle func(expanded bool)

	titleH   int
	markdown text.MarkdownStream
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

// PointerShape offers the hand over the always-toggleable title row and a
// text beam over the reasoning body.
func (t *ThinkingBlock) PointerShape(_, y int) string {
	if y >= 0 && y < t.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// CopyText returns thinking body text.
func (t *ThinkingBlock) CopyText() string { return t.Text }

// Draw renders the header — spinner + "Thinking" while streaming,
// "Thought for <span>" once done — and the themed Markdown reasoning body
// when expanded.
func (t *ThinkingBlock) Draw(ctx components.DrawContext) components.Surface {
	th := t.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	icon := "❋"
	iconSt := th.Muted
	labelSt := th.Muted
	label := "Thought"
	if t.Streaming {
		icon = "..."
		iconSt = th.ToolName
		if t.Spinner != nil {
			icon = t.Spinner.Glyph()
		}
		labelSt = th.ToolName
		label = "Thinking"
	}
	if t.Interrupted {
		icon = "⊘"
		iconSt = th.Warning
		labelSt = th.Warning
		label = "Thinking"
	}
	if !t.Streaming && !t.Interrupted && t.Duration >= time.Second {
		label = "Thought for " + components.FormatDuration(t.Duration)
	}

	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
	}
	if t.Streaming {
		if t.Model != "" {
			spans = append(spans, components.Span{Text: t.Model + " · ", Style: th.Muted})
			spans = append(spans, components.WaveLabel("thinking", th.ToolName, labelSt)...)
		} else {
			spans = append(spans, components.WaveLabel("Thinking", th.ToolName, labelSt)...)
		}
	} else {
		spans = append(spans, components.Span{Text: label, Style: labelSt})
	}
	if t.Interrupted {
		spans = append(spans, components.Span{Text: " (interrupted)", Style: th.Warning})
	}
	arrow := " ▶"
	if t.Expanded {
		arrow = " ▼"
	}
	spans = append(spans, components.Span{Text: arrow, Style: th.Muted})

	titleLines := components.WrapSpans(spans, max(w-messageIndent, 1), ctx.Method)
	t.titleH = len(titleLines)

	var bodyLines []components.RichLine
	if t.Expanded && strings.TrimSpace(t.Text) != "" {
		bodyLines = t.markdown.Render(
			t.Text,
			th,
			max(w-messageIndent, 1),
			ctx.Method,
		)
	}

	h := len(titleLines) + len(bodyLines)
	h = max(h, 1)
	s := components.NewSurface(w, h, t)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range bodyLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	// A thinking block is always expandable, so the title always lights up.
	components.HoverTitleRows(ctx, &s, t, t.titleH, th.BackgroundElement, true)
	gutterBar(&s, quietGutter(th))
	return s
}
