package block

import (
	"slices"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/session"
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
	cache     assistantRenderCache
	messageActionBar
}

// replyAnchor is how the reply's hints name the place a click would act on.
const replyAnchor = "this reply"

type assistantRenderKey struct {
	text      string
	state     session.State
	metaLabel string
	metaTail  string
	theme     components.Theme
	width     int
	method    xui.WidthMethod
}

type assistantRenderCache struct {
	key      assistantRenderKey
	lines    []components.RichLine
	markdown text.MarkdownStream
	surface  components.Surface
	valid    bool
}

func (assistantBlock *AssistantBlock) theme() components.Theme {
	if assistantBlock.Theme.Success.Fg.Kind == 0 && assistantBlock.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return assistantBlock.Theme
}

// Handle runs the action strip; the assistant text itself is read-only.
func (assistantBlock *AssistantBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	assistantBlock.handleActionMouse(ctx, ev)
}

// PointerShape offers the hand over the action strip and marks the rest of
// the output as selectable transcript text.
func (assistantBlock *AssistantBlock) PointerShape(x, y int) string {
	if shape, ok := assistantBlock.actionShape(x, y); ok {
		return shape
	}
	return components.ShapeText
}

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
	key := assistantRenderKey{
		text:      assistantBlock.Text,
		state:     assistantBlock.State,
		metaLabel: assistantBlock.MetaLabel,
		metaTail:  assistantBlock.MetaTail,
		theme:     th,
		width:     w,
		method:    ctx.Method,
	}
	// The strip is released before the render and drawn again after it, on
	// every pass, cache hit included. The cached surface outlives the frame
	// that filled it and keeps every row whose content did not change, while
	// the strip belongs to no row: released first, it leaves neither a copy
	// of itself on the row it moved off nor a hover tint from a frame the
	// pointer has long since left.
	assistantBlock.erasePainted(&assistantBlock.cache.surface)
	if !assistantBlock.cache.valid || assistantBlock.cache.key != key {
		assistantBlock.renderLines(ctx, key, th, w)
	}
	assistantBlock.layoutActions(w, ctx.Method)
	assistantBlock.paint(&assistantBlock.cache.surface, ctx, assistantBlock, th, xui.Style{})
	return assistantBlock.cache.surface
}

// layoutActions puts the strip on the last row of the reply, which is the
// end-of-turn footer when there is one and the closing line of the text
// otherwise, and starts it clear of whatever that row already says.
func (assistantBlock *AssistantBlock) layoutActions(w int, method xui.WidthMethod) {
	last := assistantBlock.cache.surface.Size.Height - 1
	assistantBlock.layout(last, w, assistantBlock.contentEnd(last, method), replyAnchor, method)
	if len(assistantBlock.strip.buttons) > 0 || last < 1 {
		return
	}
	// A footer that spells out a long model name, its context size and the
	// round's duration can fill a narrow pane's row to the edge. The buttons
	// then step one line up instead of disappearing: the row above is the
	// reply's own last line of text, and a line of prose rarely ends flush
	// right.
	above := last - 1
	assistantBlock.layout(above, w, assistantBlock.contentEnd(above, method), replyAnchor, method)
}

// contentEnd is the column where the row's own content stops.
func (assistantBlock *AssistantBlock) contentEnd(row int, method xui.WidthMethod) int {
	lines := assistantBlock.cache.lines
	if row < 0 || row >= len(lines) {
		return messageIndent
	}
	return messageIndent + components.MeasureSpans(lines[row], method)
}

// renderLines rebuilds the reply's lines and repaints the cached surface.
func (assistantBlock *AssistantBlock) renderLines(
	ctx components.DrawContext,
	key assistantRenderKey,
	th components.Theme,
	w int,
) {
	markdownLines := assistantBlock.cache.markdown.Render(
		assistantBlock.Text,
		th,
		max(w-messageIndent, 1),
		ctx.Method,
	)
	lines := append([]components.RichLine(nil), markdownLines...)
	if assistantBlock.State == session.StateError && assistantBlock.Text != "" {
		lines = append([]components.RichLine{
			{components.Span{Text: "✕ run error", Style: th.Destructive}},
			{},
		}, lines...)
	}
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
	assistantBlock.cache.updateSurface(key, lines, assistantBlock)
	assistantBlock.cache.key = key
	assistantBlock.cache.valid = true
}

func (c *assistantRenderCache) updateSurface(
	key assistantRenderKey,
	lines []components.RichLine,
	widget components.Widget,
) {
	prefix := 0
	if c.valid && c.key.width == key.width && c.key.method == key.method {
		for prefix < len(c.lines) && prefix < len(lines) && slices.Equal(c.lines[prefix], lines[prefix]) {
			prefix++
		}
	}
	height := max(len(lines), 1)
	c.resizeSurface(key.width, height, widget)
	start := min(prefix, height)
	for i := start * key.width; i < len(c.surface.Buffer); i++ {
		c.surface.Buffer[i] = xui.EmptyCell()
	}
	for y := start; y < len(lines); y++ {
		components.PaintSpans(&c.surface, messageIndent, y, lines[y], key.method)
	}
	// The assistant's own voice gets an undimmed muted bar. Painted per row
	// from the repaint prefix: earlier rows keep theirs (theme is in the key).
	for y := start; y < height; y++ {
		c.surface.Buffer[y*key.width] = xui.Cell{
			Char: gutterGlyph, Width: 1, Style: key.theme.Muted,
		}
	}
	c.lines = lines
}

func (c *assistantRenderCache) resizeSurface(width, height int, widget components.Widget) {
	required := width * height
	if c.surface.Size.Width != width || c.surface.Buffer == nil {
		c.surface = components.NewSurface(width, height, widget)
		return
	}
	oldLen := len(c.surface.Buffer)
	if required > cap(c.surface.Buffer) {
		capacity := max(required, cap(c.surface.Buffer)*2)
		buffer := make([]xui.Cell, required, capacity)
		copy(buffer, c.surface.Buffer)
		c.surface.Buffer = buffer
	} else {
		c.surface.Buffer = c.surface.Buffer[:required]
	}
	for i := oldLen; i < required; i++ {
		c.surface.Buffer[i] = xui.EmptyCell()
	}
	c.resizeChrome(required)
	c.surface.Size = components.Size{Width: width, Height: height}
	c.surface.Widget = widget
}

// resizeChrome keeps the chrome mark the same length as the buffer it
// describes. The surface survives between frames and the reply grows while
// it streams, so a mark sized for an earlier height would leave the new rows
// undescribed, and the marks of rows that went away would land on whatever
// row took their index.
func (c *assistantRenderCache) resizeChrome(required int) {
	if c.surface.Chrome == nil {
		return
	}
	switch {
	case len(c.surface.Chrome) > required:
		c.surface.Chrome = c.surface.Chrome[:required]
	case len(c.surface.Chrome) < required:
		grown := make([]bool, required)
		copy(grown, c.surface.Chrome)
		c.surface.Chrome = grown
	}
}
