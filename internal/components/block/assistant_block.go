package block

import (
	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/text"
	"github.com/pulseaiclub/phi/internal/session"
)

// AssistantBlock renders assistant Markdown (GFM) with themed typography,
// path highlights, and syntax-colored fenced code.
type AssistantBlock struct {
	Text  string
	State session.State
	// MetaLabel / MetaTail compose the end-of-turn footer row, opencode-style:
	// "▣ <MetaLabel> · <MetaTail>" — the marker in Secondary, the label in
	// Foreground, the tail muted. An empty label renders no row, and the row
	// never enters CopyText.
	MetaLabel string
	MetaTail  string
	Theme     components.Theme
}

func (assistantBlock *AssistantBlock) theme() components.Theme {
	if assistantBlock.Theme.Success.Fg.Kind == 0 && assistantBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return assistantBlock.Theme
}

// Handle is a no-op; assistant output is read-only.
func (*AssistantBlock) Handle(_ *components.EventContext, _ xui.Event) {}

// CopyText returns the assistant message body.
func (assistantBlock *AssistantBlock) CopyText() string { return assistantBlock.Text }

// Draw renders the assistant markdown with opencode-style typography:
// hanging-indent lists, ruled quotes, and boxed code.
func (assistantBlock *AssistantBlock) Draw(ctx components.DrawContext) components.Surface {
	th := assistantBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	lines := text.RenderMarkdownLines(assistantBlock.Text, th, max(w-messageIndent, 1), ctx.Method)
	if assistantBlock.State == session.StateCancelled && assistantBlock.Text != "" {
		lines = append(lines, components.RichLine{
			components.Span{Text: "cancelled", Style: th.Muted},
		})
	}
	if assistantBlock.MetaLabel != "" {
		row := []components.Span{
			{Text: "▣ ", Style: th.Secondary},
			{Text: assistantBlock.MetaLabel, Style: th.Foreground},
		}
		if assistantBlock.MetaTail != "" {
			row = append(row, components.Span{Text: " · " + assistantBlock.MetaTail, Style: th.Muted})
		}
		lines = append(lines, components.RichLine(row))
	}
	return components.PaintRichLinesAt(messageIndent, w, lines, ctx.Method, assistantBlock)
}
