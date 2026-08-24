package text

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/pulseaiclub/xui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	goldtext "github.com/yuin/goldmark/text"

	"github.com/pulseaiclub/phi/internal/components"
)

var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
).Parser()

// RenderMarkdown turns CommonMark/GFM into theme-colored spans for the TUI.
// Structural markers (#, `, *) are stripped; emphasis and block chrome are
// expressed via xui.Style and Theme semantic colors.
func RenderMarkdown(src string, th components.Theme) []components.Span {
	if src == "" {
		return nil
	}
	source := []byte(src)
	doc := mdParser.Parse(goldtext.NewReader(source))
	r := &mdRenderer{source: source, th: th}
	r.blockChildren(doc)
	if len(r.out) == 0 {
		return []components.Span{{Text: src, Style: th.Foreground}}
	}
	return r.out
}

type mdRenderer struct {
	source []byte
	th     components.Theme
	out    []components.Span

	// inline stack
	bold, italic, strike, code bool
	link                       bool
	heading                    *xui.Style
}

func (r *mdRenderer) write(s string, st xui.Style) {
	if s == "" {
		return
	}
	r.out = append(r.out, components.Span{Text: s, Style: st})
}

func (r *mdRenderer) nl() {
	r.write("\n", r.th.Foreground)
}

func (r *mdRenderer) blankLine() {
	if len(r.out) == 0 {
		return
	}
	// Avoid stacking more than one blank line between blocks.
	txt := r.out[len(r.out)-1].Text
	if strings.HasSuffix(txt, "\n\n") {
		return
	}
	if strings.HasSuffix(txt, "\n") {
		r.write("\n", r.th.Foreground)
		return
	}
	r.write("\n\n", r.th.Foreground)
}

func (r *mdRenderer) inlineStyle() xui.Style {
	st := r.th.Foreground
	if r.heading != nil {
		st = *r.heading
	}
	switch {
	case r.code:
		st = r.th.Markdown.InlineCode
	case r.link:
		st = r.th.Markdown.LinkText
	}
	// Prose emphasis carries its own color (opencode markdownStrong/Emph);
	// inside code, links, and headings only the attribute rides.
	prose := !r.code && !r.link && r.heading == nil
	if r.bold {
		st.Bold = true
		if prose {
			st.Fg = r.th.Markdown.Strong.Fg
		}
	}
	if r.italic {
		st.Italic = true
		if prose {
			st.Fg = r.th.Markdown.Emph.Fg
		}
	}
	if r.strike {
		st.Strikethrough = true
	}
	return st
}

func (r *mdRenderer) blockChildren(n ast.Node) {
	first := true
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if !first {
			switch c.Kind() {
			case ast.KindParagraph, ast.KindHeading, ast.KindFencedCodeBlock,
				ast.KindCodeBlock, ast.KindBlockquote, ast.KindList,
				ast.KindThematicBreak, east.KindTable:
				r.blankLine()
			}
		}
		first = false
		r.renderBlock(c)
	}
}

func (r *mdRenderer) renderBlock(n ast.Node) {
	switch n.Kind() {
	case ast.KindDocument:
		r.blockChildren(n)
	case ast.KindParagraph:
		r.renderInlineChildren(n)
	case ast.KindHeading:
		h := n.(*ast.Heading)
		r.renderHeading(h)
	case ast.KindBlockquote:
		r.renderBlockquote(n)
	case ast.KindList:
		r.renderList(n.(*ast.List))
	case ast.KindListItem:
		// Handled by renderList.
		r.renderListItem(n.(*ast.ListItem), "• ", r.th.Markdown.ListItem)
	case ast.KindTextBlock:
		r.renderInlineChildren(n)
	case ast.KindFencedCodeBlock:
		r.renderFencedCode(n.(*ast.FencedCodeBlock))
	case ast.KindCodeBlock:
		r.renderIndentedCode(n.(*ast.CodeBlock))
	case ast.KindThematicBreak:
		r.write("────────", r.th.Border)
	case east.KindTable:
		r.renderTable(n.(*east.Table))
	case east.KindTaskCheckBox:
		// Handled inside list item / inline.
	default:
		if n.HasChildren() {
			r.blockChildren(n)
		}
	}
}

func (r *mdRenderer) renderHeading(h *ast.Heading) {
	// opencode: every heading level shares markdownHeading bold; only H1 adds
	// an underline.
	base := r.th.Markdown.Heading
	if h.Level == 1 {
		base.Underline = true
	}
	prev := r.heading
	r.heading = &base
	r.renderInlineChildren(h)
	r.heading = prev
}

func (r *mdRenderer) renderBlockquote(n ast.Node) {
	// Collect quote body, then prefix each visual line with a rule.
	sub := &mdRenderer{source: r.source, th: r.th}
	sub.blockChildren(n)
	body := spansText(sub.out)
	lines := strings.Split(body, "\n")
	quote := r.th.Markdown.BlockQuote
	rule := r.th.Border
	for i, line := range lines {
		if i > 0 {
			r.nl()
		}
		r.write("▎ ", rule)
		if line != "" {
			r.write(line, quote)
		}
	}
}

func (r *mdRenderer) renderList(list *ast.List) {
	i := 0
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		if i > 0 {
			r.nl()
		}
		marker := "• "
		markerSt := r.th.Markdown.ListItem
		if list.IsOrdered() {
			num := list.Start + i
			marker = fmt.Sprintf("%d. ", num)
			markerSt = r.th.Markdown.ListEnum
		}
		r.renderListItem(item, marker, markerSt)
		i++
	}
}

func (r *mdRenderer) renderListItem(item *ast.ListItem, marker string, markerSt xui.Style) {
	r.write(marker, markerSt)

	// Task checkbox as first child of paragraph.
	if p := item.FirstChild(); p != nil {
		if tb := firstTaskBox(p); tb != nil {
			if tb.IsChecked {
				r.write("☑ ", r.th.Success)
			} else {
				r.write("☐ ", r.th.Muted)
			}
		}
	}

	// Render item body; indent continuation lines conceptually via soft wrap
	// (caller wraps spans). Nested lists get a newline + indent.
	first := true
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		if !first {
			r.nl()
			r.write("  ", r.th.Foreground)
		}
		first = false
		switch c.Kind() {
		case ast.KindList:
			r.renderList(c.(*ast.List))
		case ast.KindParagraph, ast.KindTextBlock:
			r.renderInlineChildren(c)
		default:
			r.renderBlock(c)
		}
	}
}

func firstTaskBox(n ast.Node) *east.TaskCheckBox {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if tb, ok := c.(*east.TaskCheckBox); ok {
			return tb
		}
	}
	return nil
}

func (r *mdRenderer) renderFencedCode(n *ast.FencedCodeBlock) {
	lang := ""
	if n.Info != nil {
		lang = strings.TrimSpace(string(n.Info.Segment.Value(r.source)))
		if i := strings.IndexAny(lang, " \t"); i >= 0 {
			lang = lang[:i]
		}
	}
	code := codeBlockString(n, r.source)
	r.renderCodeBlock(code, lang)
}

func (r *mdRenderer) renderIndentedCode(n *ast.CodeBlock) {
	r.renderCodeBlock(codeBlockString(n, r.source), "")
}

func (r *mdRenderer) renderCodeBlock(code, lang string) {
	code = strings.TrimRight(code, "\n")
	// Language caption only — no box or rule chrome (those get copied with the code).
	if lang != "" {
		r.write(lang, r.th.Muted)
		r.nl()
	}
	lines := highlightCodeLines(code, lang, r.th)
	if len(lines) == 0 {
		lines = [][]components.Span{{}}
	}
	for i, line := range lines {
		if i > 0 {
			r.nl()
		}
		r.out = append(r.out, line...)
	}
}

// highlightCodeLines syntax-highlights a code block, returning one span slice per line.
func highlightCodeLines(code, lang string, th components.Theme) [][]components.Span {
	if code == "" {
		return [][]components.Span{{}}
	}
	fallback := func() [][]components.Span {
		raw := strings.Split(code, "\n")
		out := make([][]components.Span, len(raw))
		for i, line := range raw {
			if line == "" {
				out[i] = nil
				continue
			}
			out[i] = []components.Span{{Text: line, Style: th.Markdown.CodeBlock}}
		}
		return out
	}
	if lang == "" {
		return fallback()
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		return fallback()
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return fallback()
	}
	var lines [][]components.Span
	var cur []components.Span
	flush := func() {
		lines = append(lines, cur)
		cur = nil
	}
	for _, tok := range it.Tokens() {
		st := chromaStyle(tok.Type, th)
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				flush()
			}
			if part != "" {
				cur = append(cur, components.Span{Text: part, Style: st})
			}
		}
	}
	flush()
	// Token stream often ends with a trailing empty line after final \n.
	if len(lines) > 1 && len(lines[len(lines)-1]) == 0 && strings.HasSuffix(code, "\n") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return fallback()
	}
	return lines
}

// chromaStyle maps a chroma token to the theme's syntax roles, following the
// opencode scope rules (builtins land on Variable, which upstream paints the
// error red).
func chromaStyle(t chroma.TokenType, th components.Theme) xui.Style {
	sy := th.Syntax
	switch {
	case t.InCategory(chroma.Comment), t.InCategory(chroma.CommentPreproc):
		return sy.Comment
	case t.InCategory(chroma.Keyword), t.InCategory(chroma.KeywordType):
		return sy.Keyword
	case t.InCategory(chroma.String), t.InCategory(chroma.LiteralString):
		return sy.String
	case t.InCategory(chroma.LiteralNumber), t.InCategory(chroma.LiteralDate):
		return sy.Number
	case t.InCategory(chroma.NameFunction):
		return sy.Function
	case t.InCategory(chroma.NameClass):
		return sy.Type
	case t.InCategory(chroma.NameBuiltin), t.InCategory(chroma.NameDecorator):
		return sy.Variable
	case t.InCategory(chroma.Operator):
		return sy.Operator
	case t.InCategory(chroma.Punctuation):
		return sy.Punctuation
	case t.InCategory(chroma.Error):
		return th.Destructive
	default:
		return th.Foreground
	}
}

func codeBlockString(n ast.Node, source []byte) string {
	var b strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		line := n.Lines().At(i)
		b.Write(line.Value(source))
	}
	return b.String()
}

func (r *mdRenderer) renderTable(t *east.Table) {
	var rows [][]string
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			sub := &mdRenderer{source: r.source, th: r.th}
			sub.renderInlineChildren(cell)
			cells = append(cells, strings.TrimSpace(spansText(sub.out)))
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	widths := make([]int, cols)
	for _, row := range rows {
		for i, c := range row {
			if w := xui.StringWidth(c, xui.WidthUnicode); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for ri, row := range rows {
		if ri > 0 {
			r.nl()
		}
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			pad := widths[i] - xui.StringWidth(cell, xui.WidthUnicode)
			pad = max(pad, 0)
			st := r.th.Foreground
			if ri == 0 {
				st.Bold = true
				st.Fg = r.th.ToolName.Fg
			}
			if i > 0 {
				r.write(" │ ", r.th.Border)
			}
			r.write(cell+strings.Repeat(" ", pad), st)
		}
		if ri == 0 {
			r.nl()
			for i := 0; i < cols; i++ {
				if i > 0 {
					r.write("─┼─", r.th.Border)
				}
				r.write(strings.Repeat("─", widths[i]), r.th.Border)
			}
		}
	}
}

func (r *mdRenderer) renderInlineChildren(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.renderInline(c)
	}
}

func (r *mdRenderer) renderInline(n ast.Node) {
	switch n.Kind() {
	case ast.KindText:
		t := n.(*ast.Text)
		seg := string(t.Segment.Value(r.source))
		if r.code || r.link {
			r.write(seg, r.inlineStyle())
		} else {
			r.out = append(r.out, highlightPathsStyled(seg, r.inlineStyle())...)
		}
		// Chat UX: treat soft breaks as hard newlines so model output like
		// "a\nb" keeps its line structure instead of collapsing to a space.
		if t.SoftLineBreak() || t.HardLineBreak() {
			r.nl()
		}
	case ast.KindString:
		s := n.(*ast.String)
		r.write(string(s.Value), r.inlineStyle())
	case ast.KindCodeSpan:
		prev := r.code
		r.code = true
		r.renderInlineChildren(n)
		r.code = prev
	case ast.KindEmphasis:
		em := n.(*ast.Emphasis)
		if em.Level >= 2 {
			prev := r.bold
			r.bold = true
			r.renderInlineChildren(n)
			r.bold = prev
		} else {
			prev := r.italic
			r.italic = true
			r.renderInlineChildren(n)
			r.italic = prev
		}
	case east.KindStrikethrough:
		prev := r.strike
		r.strike = true
		r.renderInlineChildren(n)
		r.strike = prev
	case ast.KindLink:
		prev := r.link
		r.link = true
		r.renderInlineChildren(n)
		r.link = prev
	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		prev := r.link
		r.link = true
		r.write(string(al.Label(r.source)), r.inlineStyle())
		r.link = prev
	case ast.KindImage:
		img := n.(*ast.Image)
		r.write("🖼 ", r.th.Muted)
		prev := r.link
		r.link = true
		r.renderInlineChildren(img)
		r.link = prev
	case east.KindTaskCheckBox:
		// Already rendered at list-item level.
	case ast.KindRawHTML:
		// Skip HTML for TUI safety.
	default:
		if n.HasChildren() {
			r.renderInlineChildren(n)
		}
	}
}

// highlightPathsStyled path-highlights plain text: paths keep the base style
// and gain an underline, so prose color stays even (opencode paints plain
// text uniformly).
func highlightPathsStyled(text string, base xui.Style) []components.Span {
	var out []components.Span
	last := 0
	for _, m := range rePathish.FindAllStringIndex(text, -1) {
		tok := text[m[0]:m[1]]
		if !looksHighlightable(tok) {
			continue
		}
		if m[0] > last {
			out = append(out, components.Span{Text: text[last:m[0]], Style: base})
		}
		// Paths stay in the prose color — opencode paints plain text evenly;
		// the underline is the whole affordance.
		pathSt := base
		pathSt.Underline = true
		out = append(out, components.Span{Text: tok, Style: pathSt})
		last = m[1]
	}
	if last < len(text) {
		out = append(out, components.Span{Text: text[last:], Style: base})
	}
	if len(out) == 0 && text != "" {
		out = []components.Span{{Text: text, Style: base}}
	}
	return out
}

func spansText(spans []components.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}
