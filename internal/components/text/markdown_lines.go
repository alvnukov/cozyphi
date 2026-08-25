package text

import (
	"strings"

	"github.com/pulseaiclub/xui"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	goldtext "github.com/yuin/goldmark/text"

	"github.com/alvnukov/cozyphi/internal/components"
)

// RenderMarkdownLines renders CommonMark/GFM into final display lines with
// opencode-style typography: word-wrapped paragraphs, hanging-indent lists,
// blockquotes ruled on every line, and fenced code in a rounded box sized to
// its content. Being width-aware lets block chrome and wrapping agree on
// geometry — callers paint the lines as-is.
func RenderMarkdownLines(src string, th components.Theme, width int, method xui.WidthMethod) []components.RichLine {
	if src == "" {
		return nil
	}
	if width < 1 {
		width = 1
	}
	source := []byte(src)
	doc := mdParser.Parse(goldtext.NewReader(source))
	r := &linesRenderer{source: source, th: th, width: width, method: method}
	r.blockChildren(doc)
	return r.lines
}

type linesRenderer struct {
	source []byte
	th     components.Theme
	width  int
	method xui.WidthMethod
	lines  []components.RichLine
}

// sub renders a node's inline content into spans through the shared
// span-level markdown renderer (theme colors, stripped markers).
func (r *linesRenderer) sub() *mdRenderer {
	return &mdRenderer{source: r.source, th: r.th}
}

func (r *linesRenderer) addWrapped(spans []components.Span) {
	r.lines = append(r.lines, components.WrapSpans(spans, r.width, r.method)...)
}

func (r *linesRenderer) indentStr(n int) components.Span {
	return components.Span{Text: strings.Repeat(" ", n), Style: r.th.Foreground}
}

func (r *linesRenderer) blockChildren(n ast.Node) {
	first := true
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if !first {
			switch c.Kind() {
			case ast.KindParagraph, ast.KindHeading, ast.KindFencedCodeBlock,
				ast.KindCodeBlock, ast.KindBlockquote, ast.KindList,
				ast.KindThematicBreak, east.KindTable:
				r.lines = append(r.lines, nil)
			}
		}
		first = false
		r.renderBlock(c)
	}
}

func (r *linesRenderer) renderBlock(n ast.Node) {
	switch n.Kind() {
	case ast.KindDocument:
		r.blockChildren(n)
	case ast.KindParagraph, ast.KindTextBlock:
		sub := r.sub()
		sub.renderInlineChildren(n)
		r.addWrapped(sub.out)
	case ast.KindHeading:
		sub := r.sub()
		sub.renderHeading(n.(*ast.Heading))
		r.addWrapped(sub.out)
	case ast.KindBlockquote:
		r.renderBlockquote(n)
	case ast.KindList:
		r.renderList(n.(*ast.List), 0)
	case ast.KindFencedCodeBlock:
		r.renderCodeBox(
			codeBlockString(n.(*ast.FencedCodeBlock), r.source),
			fenceLang(n.(*ast.FencedCodeBlock), r.source),
		)
	case ast.KindCodeBlock:
		r.renderCodeBox(codeBlockString(n.(*ast.CodeBlock), r.source), "")
	case ast.KindThematicBreak:
		r.lines = append(r.lines, components.RichLine{
			components.Span{Text: strings.Repeat("─", min(r.width, 80)), Style: r.th.Border},
		})
	case east.KindTable:
		sub := r.sub()
		sub.renderTable(n.(*east.Table))
		r.addWrapped(sub.out)
	default:
		if n.HasChildren() {
			r.blockChildren(n)
		}
	}
}

// renderBlockquote wraps the quote body, then rules every visual line so a
// soft-wrapped quote keeps its bar past row one.
func (r *linesRenderer) renderBlockquote(n ast.Node) {
	sub := r.sub()
	sub.blockChildren(n)
	wrapped := components.WrapSpans(sub.out, max(r.width-2, 1), r.method)
	for _, line := range wrapped {
		ruled := append(components.RichLine{components.Span{Text: "▎ ", Style: r.th.Border}}, line...)
		r.lines = append(r.lines, ruled)
	}
}

// renderList lays out items with hanging indent: the marker owns the first
// line's lead, continuation lines align under the item body, nested lists
// indent one marker width deeper.
func (r *linesRenderer) renderList(list *ast.List, indent int) {
	i := 0
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		if i > 0 {
			r.lines = append(r.lines, nil)
		}
		marker := "• "
		markerSt := r.th.Markdown.ListItem
		if list.IsOrdered() {
			marker = itoa(list.Start+i) + ". "
			markerSt = r.th.Markdown.ListEnum
		}
		markerW := xui.StringWidth(marker, r.method)
		body, nested := r.itemBody(item)
		wrapped := components.WrapSpans(body, max(r.width-indent-markerW, 1), r.method)
		for j, line := range wrapped {
			var out components.RichLine
			if j == 0 {
				out = append(out, r.indentStr(indent), components.Span{Text: marker, Style: markerSt})
			} else {
				out = append(out, r.indentStr(indent+markerW))
			}
			r.lines = append(r.lines, append(out, line...))
		}
		if nested != nil {
			r.renderList(nested.(*ast.List), indent+2)
		}
		i++
	}
}

// itemBody renders an item's non-list children (and task checkbox) into
// spans; a nested list is returned separately for recursion.
func (r *linesRenderer) itemBody(item *ast.ListItem) (body []components.Span, nested ast.Node) {
	first := true
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == ast.KindList {
			nested = c
			continue
		}
		sub := r.sub()
		if p, ok := c.(*ast.Paragraph); ok {
			if tb := firstTaskBox(p); tb != nil && first {
				if tb.IsChecked {
					sub.write("☑ ", r.th.Success)
				} else {
					sub.write("☐ ", r.th.Muted)
				}
			}
		}
		switch c.Kind() {
		case ast.KindParagraph, ast.KindTextBlock:
			sub.renderInlineChildren(c)
		default:
			sub.renderBlock(c)
		}
		if !first {
			body = append(body, components.Span{Text: "\n\n", Style: r.th.Foreground})
		}
		body = append(body, sub.out...)
		first = false
	}
	return body, nested
}

// renderCodeBox draws fenced code in a rounded box: language embedded in the
// top border, one space of padding, content-sized but clamped to the width.
func (r *linesRenderer) renderCodeBox(code, lang string) {
	// Tabs are invisible geometry in a terminal cell grid; spaces are honest.
	code = strings.ReplaceAll(strings.TrimRight(code, "\n"), "\t", "    ")
	inner := max(r.width-4, 1)
	var codeLines []components.RichLine
	contentW := 0
	for _, l := range highlightCodeLines(code, lang, r.th) {
		for _, wl := range components.WrapSpans(l, inner, r.method) {
			if w := components.MeasureSpans(wl, r.method); w > contentW {
				contentW = w
			}
			codeLines = append(codeLines, wl)
		}
	}
	boxInner := contentW
	if lang != "" {
		boxInner = max(boxInner, len(lang)+3)
	}
	boxInner = max(boxInner, 1)

	r.lines = append(r.lines, r.boxBorderLine("╭", "╮", boxInner, lang))
	pad := strings.Repeat(" ", boxInner)
	for _, line := range codeLines {
		fill := max(boxInner-components.MeasureSpans(line, r.method), 0)
		boxed := components.RichLine{components.Span{Text: "│ ", Style: r.th.Border}}
		boxed = append(boxed, line...)
		boxed = append(boxed, components.Span{Text: pad[:fill] + " │", Style: r.th.Border})
		r.lines = append(r.lines, boxed)
	}
	r.lines = append(r.lines, r.boxBorderLine("╰", "╯", boxInner, ""))
}

func (r *linesRenderer) boxBorderLine(left, right string, boxInner int, lang string) components.RichLine {
	inner := boxInner + 2 // one space of padding on each side
	var b strings.Builder
	b.WriteString(left)
	if lang != "" {
		b.WriteString("─ ")
		b.WriteString(lang)
		b.WriteString(" ")
		inner -= len(lang) + 3
	}
	if inner > 0 {
		b.WriteString(strings.Repeat("─", inner))
	}
	b.WriteString(right)
	return components.RichLine{components.Span{Text: b.String(), Style: r.th.Border}}
}

// fenceLang extracts the info string's first word as the code language.
func fenceLang(n *ast.FencedCodeBlock, source []byte) string {
	if n.Info == nil {
		return ""
	}
	lang := strings.TrimSpace(string(n.Info.Segment.Value(source)))
	if i := strings.IndexAny(lang, " \t"); i >= 0 {
		lang = lang[:i]
	}
	return lang
}

func itoa(n int) string {
	if n < 0 {
		return "0"
	}
	digits := []byte{}
	for {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
		if n == 0 {
			return string(digits)
		}
	}
}
