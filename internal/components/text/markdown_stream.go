package text

import (
	"strings"

	"github.com/pulseaiclub/xui"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	goldtext "github.com/yuin/goldmark/text"

	"github.com/alvnukov/cozyphi/internal/components"
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

// stableMarkdownCut returns the source offset where the reparsed tail may
// begin: the end of the longest leading run of top-level blocks that future
// input cannot change. The final top-level block always stays in the tail
// (future input extends it), and so does any earlier block containing a
// bracket group a later reference definition could still turn into a link —
// that block stops the run instead of freezing every block after it.
func stableMarkdownCut(src string) int {
	if src == "" {
		return 0
	}
	source := []byte(src)
	doc := mdParser.Parse(goldtext.NewReader(source))
	if doc.ChildCount() < 2 || !supportedStreamDocument(doc) {
		return 0
	}
	// starts[i] is the source offset of top-level block i; starts[i+1] doubles
	// as the exclusive end of block i. An unmeasurable block stops collection
	// — the commit loop then never reaches it, failing closed.
	starts := make([]int, 0, doc.ChildCount())
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		start, ok := markdownNodeStart(node, source)
		if !ok {
			break
		}
		starts = append(starts, start)
	}
	cut := 0
	for i := 0; i+1 < len(starts); i++ {
		if unresolvedReference(src[starts[i]:starts[i+1]]) {
			break
		}
		cut = lineStart(source, starts[i+1])
	}
	return cut
}

// unresolvedReference reports whether src contains a bracket group that a
// reference definition appended later could still turn into a link: the
// closing "]" is not followed by "(", the inline-link form that carries its
// own target. Full and collapsed reference links resolve against definitions
// from anywhere in the document, so they count as unresolved here.
func unresolvedReference(src string) bool {
	for i := 0; i < len(src); i++ {
		if src[i] != '[' {
			continue
		}
		end := strings.IndexByte(src[i:], ']')
		if end < 0 {
			return true
		}
		if next := i + end + 1; next >= len(src) || src[next] != '(' {
			return true
		}
		i += end
	}
	return false
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
