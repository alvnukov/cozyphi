package block

import (
	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

// CompactionBlock is a transcript marker shown after context compaction
// (Cursor-style italic "Compacted").
type CompactionBlock struct {
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

// Draw renders the italic "Compacted" marker line.
func (b *CompactionBlock) Draw(ctx components.DrawContext) components.Surface {
	th := b.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	st := th.Muted
	st.Italic = true
	st.Dim = true
	lines := components.WrapSpans([]components.Span{
		{Text: "Compacted", Style: st},
	}, w, ctx.Method)
	return components.PaintRichLines(w, lines, ctx.Method, b)
}
