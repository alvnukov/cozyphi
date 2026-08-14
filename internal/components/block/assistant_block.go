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
	Theme components.Theme
}

func (assistantBlock *AssistantBlock) theme() components.Theme {
	if assistantBlock.Theme.Success.Fg.Kind == 0 && assistantBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return assistantBlock.Theme
}

func (_ *AssistantBlock) Handle(_ *components.EventContext, _ xui.Event) {}

// CopyText returns the assistant message body.
func (assistantBlock *AssistantBlock) CopyText() string { return assistantBlock.Text }

func (assistantBlock *AssistantBlock) Draw(ctx components.DrawContext) components.Surface {
	th := assistantBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	spans := text.RenderMarkdown(assistantBlock.Text, th)
	if assistantBlock.State == session.StateCancelled && assistantBlock.Text != "" {
		if len(spans) > 0 {
			spans = append(spans, components.Span{Text: "\n", Style: th.Muted})
		}
		spans = append(spans, components.Span{Text: "cancelled", Style: th.Muted})
	}
	return components.PaintRichLines(w, components.WrapSpans(spans, w, ctx.Method), ctx.Method, assistantBlock)
}
