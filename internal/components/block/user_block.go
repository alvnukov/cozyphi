package block

import (
	"github.com/pulseaiclub/xui"

	components "github.com/alvnukov/cozyphi/internal/components"
)

// UserBlock renders a user prompt with an accent left bar (opencode style).
type UserBlock struct {
	Text  string
	Theme components.Theme
	messageActionBar
}

// promptAnchor is how the prompt's hints name the place a click would act on.
const promptAnchor = "this prompt"

func (userBlock *UserBlock) theme() components.Theme {
	if userBlock.Theme.Success.Fg.Kind == 0 && userBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return userBlock.Theme
}

// Handle runs the action strip; the prompt body itself is not interactive.
func (userBlock *UserBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	userBlock.handleActionMouse(ctx, ev)
}

// PointerShape offers the hand over the action strip and marks the rest of
// the prompt as selectable transcript text.
func (userBlock *UserBlock) PointerShape(x, y int) string {
	if shape, ok := userBlock.actionShape(x, y); ok {
		return shape
	}
	return components.ShapeText
}

// CopyText returns the prompt body (without the left rule).
func (userBlock *UserBlock) CopyText() string { return userBlock.Text }

// Draw renders the prompt as an opencode UserMessage panel: a full-height
// secondary ┃ rule, a panel background filling every cell right of it, one
// blank panel row above and below the text, and the text inset two columns.
func (userBlock *UserBlock) Draw(ctx components.DrawContext) components.Surface {
	th := userBlock.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	innerW := w - 3 // rule column + two padding columns
	innerW = max(innerW, 1)
	lines := components.WrapSpans([]components.Span{{Text: userBlock.Text, Style: th.Foreground}}, innerW, ctx.Method)
	h := len(lines) + 2 // padding rows top and bottom
	s := components.NewSurface(w, h, userBlock)
	for y := range h {
		// ┃ tiles full cell height; "|" leaves gaps between wrapped rows.
		s.SetCell(0, y, xui.Cell{Char: "┃", Width: 1, Style: th.Secondary})
		for x := 1; x < w; x++ {
			s.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: th.BackgroundPanel})
		}
	}
	for i, line := range lines {
		components.PaintSpans(&s, 3, i+1, line, ctx.Method)
	}
	// The strip rides the blank padding row above the text, which is panel
	// all the way across and belongs to no line of the prompt, so the block
	// keeps the height it had before it had buttons.
	userBlock.layout(0, w, 3, promptAnchor, ctx.Method)
	userBlock.paint(&s, ctx, userBlock, th, th.BackgroundPanel)
	return s
}
