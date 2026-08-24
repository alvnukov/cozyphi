package text

import (
	"strings"

	"github.com/pulseaiclub/xui"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	goldtext "github.com/yuin/goldmark/text"

	"github.com/pulseaiclub/phi/internal/components"
)

// MarkdownStream incrementally lays out append-only Markdown. Completed
// top-level blocks become immutable; only the final block is reparsed while a
// response grows. Non-append edits and unsupported cross-block constructs reset
// to the exact full renderer.
type MarkdownStream struct {
	source string
	tail   string
	stable []components.RichLine
	key    markdownStreamKey
	valid  bool
}

type markdownStreamKey struct {
	theme  components.Theme
	width  int
	method xui.WidthMethod
}

// Render returns the same layout as RenderMarkdownLines while reusing stable
// top-level blocks across append-only updates.
func (s *MarkdownStream) Render(
	src string,
	theme components.Theme,
	width int,
	method xui.WidthMethod,
) []components.RichLine {
	if width < 1 {
		width = 1
	}
	key := markdownStreamKey{theme: theme, width: width, method: method}
	if !s.valid || s.key != key || len(src) < len(s.source) || !strings.HasPrefix(src, s.source) {
		s.source = src
		s.tail = src
		s.stable = nil
		s.key = key
		s.valid = true
	} else if len(src) > len(s.source) {
		s.tail += src[len(s.source):]
		s.source = src
	}

	s.commitStable(theme, width, method)
	tailLines := RenderMarkdownLines(s.tail, theme, width, method)
	return joinMarkdownBlocks(s.stable, tailLines)
}

func (s *MarkdownStream) commitStable(theme components.Theme, width int, method xui.WidthMethod) {
	cut := stableMarkdownCut(s.tail)
	if cut <= 0 {
		return
	}
	completed := s.tail[:cut]
	// Reference definitions can change the meaning of earlier [label] text and
	// must stay in the reparsed tail. Inline links are rare enough that this
	// conservative guard is preferable to stale rendering.
	if strings.Contains(completed, "[") {
		return
	}
	lines := RenderMarkdownLines(completed, theme, width, method)
	s.stable = joinMarkdownBlocks(s.stable, lines)
	s.tail = s.tail[cut:]
}

func joinMarkdownBlocks(prefix, suffix []components.RichLine) []components.RichLine {
	if len(prefix) == 0 {
		return append([]components.RichLine(nil), suffix...)
	}
	if len(suffix) == 0 {
		return append([]components.RichLine(nil), prefix...)
	}
	out := make([]components.RichLine, 0, len(prefix)+1+len(suffix))
	out = append(out, prefix...)
	out = append(out, nil)
	out = append(out, suffix...)
	return out
}

// stableMarkdownCut returns the source offset of the final top-level block.
// Everything before it can be rendered once because future input can only
// extend that final block. Unknown block kinds fail closed to a full tail.
func stableMarkdownCut(src string) int {
	if src == "" {
		return 0
	}
	source := []byte(src)
	doc := mdParser.Parse(goldtext.NewReader(source))
	if doc.ChildCount() < 2 || !supportedStreamDocument(doc) {
		return 0
	}
	start, ok := markdownNodeStart(doc.LastChild(), source)
	if !ok {
		return 0
	}
	return lineStart(source, start)
}

func supportedStreamDocument(doc ast.Node) bool {
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		switch node.Kind() {
		case ast.KindParagraph, ast.KindHeading, ast.KindFencedCodeBlock,
			ast.KindCodeBlock, ast.KindBlockquote, ast.KindList,
			ast.KindThematicBreak, east.KindTable:
		default:
			return false
		}
	}
	return true
}

func markdownNodeStart(node ast.Node, source []byte) (int, bool) {
	start := len(source)
	found := false
	var visit func(ast.Node)
	visit = func(current ast.Node) {
		if current.Type() != ast.TypeInline {
			segments := current.Lines()
			for i := 0; i < segments.Len(); i++ {
				segment := segments.At(i)
				if segment.Start < start {
					start = segment.Start
					found = true
				}
			}
		}
		if fenced, ok := current.(*ast.FencedCodeBlock); ok && fenced.Info != nil && fenced.Info.Segment.Start < start {
			start = fenced.Info.Segment.Start
			found = true
		}
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(node)
	if !found {
		return 0, false
	}
	if fenced, ok := node.(*ast.FencedCodeBlock); ok && fenced.Info == nil {
		contentLine := lineStart(source, start)
		if contentLine == 0 {
			return 0, false
		}
		start = lineStart(source, contentLine-1)
	}
	return start, true
}

func lineStart(source []byte, offset int) int {
	offset = min(max(offset, 0), len(source))
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}
