package block

import (
	"github.com/pulseaiclub/xui"

	components "github.com/pulseaiclub/phi/internal/components"
)

// UserBlock renders a user prompt with an accent left bar (opencode style).
type UserBlock struct {
	Text  string
	Theme components.Theme
}

func (userBlock *UserBlock) theme() components.Theme {
	if userBlock.Theme.Success.Fg.Kind == 0 && userBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return userBlock.Theme
}

// Handle is a no-op; the user prompt is not interactive.
func (*UserBlock) Handle(_ *components.EventContext, _ xui.Event) {}

// CopyText returns the prompt body (without the left rule).
func (userBlock *UserBlock) CopyText() string { return userBlock.Text }

// Draw renders the prompt text beside a full-height accent bar.
func (userBlock *UserBlock) Draw(ctx components.DrawContext) components.Surface {
	th := userBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	body := th.Foreground
	bar := th.Accent
	bar.Underline = false
	innerW := w - 2
	innerW = max(innerW, 1)
	lines := components.WrapSpans([]components.Span{{Text: userBlock.Text, Style: body}}, innerW, ctx.Method)
	h := len(lines)
	h = max(h, 1)
	s := components.NewSurface(w, h, userBlock)
	for y, line := range lines {
		// ┃ tiles full cell height; "|" leaves gaps between wrapped rows.
		s.SetCell(0, y, xui.Cell{Char: "┃", Width: 1, Style: bar})
		components.PaintSpans(&s, 2, y, line, ctx.Method)
	}
	return s
}
