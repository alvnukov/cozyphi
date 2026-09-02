package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// CompactionBlock is a transcript marker shown after context compaction: a
// full-width rule with a centered report title, opencode-style. When the
// compaction carries a summary the row expands — Enter or a click on the
// rule — to the themed Markdown summary, like thinking blocks.
type CompactionBlock struct {
	Text string
	// Summary is the compaction summarize body; empty on legacy rows, which
	// stay a plain inert rule.
	Summary  string
	Expanded bool
	Theme    components.Theme
	OnToggle func(expanded bool)

	titleH int
}

func (b *CompactionBlock) theme() components.Theme {
	if b.Theme.Muted.Fg.Kind == 0 && b.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return b.Theme
}

// Handle toggles expansion on Enter/space or a left-click on the rule row;
// rows without a summary consume nothing.
func (b *CompactionBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if strings.TrimSpace(b.Summary) == "" {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			b.toggle(ctx)
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < b.titleH {
			b.toggle(ctx)
		}
	}
}

// PointerShape offers the hand exactly where a click acts — the title row of
// a compaction with a summary — and a text beam over the rest.
func (b *CompactionBlock) PointerShape(_, y int) string {
	if strings.TrimSpace(b.Summary) != "" && y >= 0 && y < b.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

func (b *CompactionBlock) toggle(ctx *components.EventContext) {
	b.Expanded = !b.Expanded
	if b.OnToggle != nil {
		b.OnToggle(b.Expanded)
	}
	ctx.ConsumeAndRedraw()
}

// Draw renders the centered rule row in the border color — the report as the
// label, plus a ▶/▼ affordance when a summary can expand — and the themed
// Markdown summary below it when expanded.
func (b *CompactionBlock) Draw(ctx components.DrawContext) components.Surface {
	th := b.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	label := strings.TrimSpace(b.Text)
	if label == "" {
		label = "Compaction"
	}
	expandable := strings.TrimSpace(b.Summary) != ""
	suffix := ""
	if expandable {
		suffix = " ▶"
		if b.Expanded {
			suffix = " ▼"
		}
	}
	row := centeredRule(" "+label+suffix+" ", w, ctx.Method)
	b.titleH = 1

	var bodyLines []components.RichLine
	if expandable && b.Expanded {
		bodyLines = text.RenderMarkdownLines(
			strings.TrimSpace(b.Summary),
			th,
			max(w-messageIndent, 1),
			ctx.Method,
		)
	}

	s := components.NewSurface(w, 1+len(bodyLines), b)
	components.PaintSpans(&s, 0, 0, []components.Span{{Text: row, Style: th.Border}}, ctx.Method)
	if components.Hovering(ctx, b) && strings.TrimSpace(b.Summary) != "" {
		components.ApplyHoverRows(&s, 0, b.titleH, th.BackgroundElement)
	}
	y := 1
	for _, line := range bodyLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	return s
}

// centeredRule fills width with dashes around a centered label; widths too
// narrow for the label degrade to a plain rule.
func centeredRule(label string, w int, method xui.WidthMethod) string {
	if w <= 0 {
		return ""
	}
	labelWidth := xui.StringWidth(label, method)
	if w < labelWidth {
		return strings.Repeat("─", w)
	}
	pad := (w - labelWidth) / 2
	return strings.Repeat("─", pad) + label + strings.Repeat("─", w-labelWidth-pad)
}
