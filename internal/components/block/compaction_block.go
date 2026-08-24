package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

// CompactionBlock is a transcript marker shown after context compaction: a
// full-width rule with a centered "Compaction" title, opencode-style.
type CompactionBlock struct {
	Text  string
	Theme components.Theme
}

func (b *CompactionBlock) theme() components.Theme {
	if b.Theme.Muted.Fg.Kind == 0 && b.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return b.Theme
}

// Handle is a no-op; the compaction marker is not interactive.
func (*CompactionBlock) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the centered rule row in the border color.
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
	row := centeredRule(" "+label+" ", w, ctx.Method)
	s := components.NewSurface(w, 1, b)
	components.PaintSpans(&s, 0, 0, []components.Span{{Text: row, Style: th.Border}}, ctx.Method)
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
