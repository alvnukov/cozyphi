package block

import (
	"math"
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
			spans = append(spans, waveLabelRunes("thinking", th.ToolName, labelSt)...)
		} else {
			spans = append(spans, waveLabelRunes("Thinking", th.ToolName, labelSt)...)
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
	return s
}

const waveTick = 80 * time.Millisecond

// waveLabelRunes renders a label with a time-driven bright band sweeping
// across its letters (claude-code style). The phase comes from the wall
// clock, so the animation is fps-independent instead of frame-counted.
func waveLabelRunes(label string, accent, base xui.Style) []components.Span {
	letters := []rune(label)
	n := len(letters)
	if n == 0 {
		return nil
	}
	phase := int(time.Now().UnixNano()/int64(waveTick)) % n
	out := make([]components.Span, 0, n)
	for i, r := range letters {
		dist := ((i-phase)%n + n) % n
		t := (math.Cos(float64(dist)/float64(n)*2*math.Pi) + 1) / 2
		st := base
		if c, ok := blendColor(base.Fg, accent.Fg, t); ok {
			st.Fg = c
		} else if dist == 0 {
			st = accent
		}
		out = append(out, components.Span{Text: string(r), Style: st})
	}
	return out
}

// blendColor linearly mixes two RGB colors by t (clamped to [0,1]). It
// returns false when either color is not truecolor, so callers fall back to
// a hard highlight.
func blendColor(a, b xui.Color, t float64) (xui.Color, bool) {
	if a.Kind != xui.ColorRGB || b.Kind != xui.ColorRGB {
		return xui.Color{}, false
	}
	if t <= 0 {
		return a, true
	}
	if t >= 1 {
		return b, true
	}
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return xui.RGBColor(lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)), true
}
