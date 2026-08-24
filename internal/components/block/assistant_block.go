package block

import (
	"slices"

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
	cache     assistantRenderCache
}

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

// Handle is a no-op; assistant output is read-only.
func (*AssistantBlock) Handle(_ *components.EventContext, _ xui.Event) {}

// PointerShape marks the output as selectable transcript text.
func (*AssistantBlock) PointerShape(_, _ int) string { return components.ShapeText }

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
	if assistantBlock.cache.valid && assistantBlock.cache.key == key {
		return assistantBlock.cache.surface
	}

	markdownLines := assistantBlock.cache.markdown.Render(
		assistantBlock.Text,
		th,
		max(w-messageIndent, 1),
		ctx.Method,
	)
	lines := append([]components.RichLine(nil), markdownLines...)
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
	return assistantBlock.cache.surface
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
	c.surface.Size = components.Size{Width: width, Height: height}
	c.surface.Widget = widget
}
