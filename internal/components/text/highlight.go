package text

import (
	"regexp"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

var (
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	rePathish    = regexp.MustCompile(`\b[\w./-]+/(?:[\w./-]*/)?|\b[\w-]+\.(?:go|ts|js|md|json|zig)\b`)
)

// HighlightAssistant splits text into styled spans with inline-code coloring
// and underlined paths.
func HighlightAssistant(text string, th components.Theme) []components.Span {
	// Split by inline code first, then path-highlight the rest.
	var out []components.Span
	last := 0
	for _, m := range reInlineCode.FindAllStringSubmatchIndex(text, -1) {
		if m[0] > last {
			out = append(out, highlightPaths(text[last:m[0]], th)...)
		}
		out = append(out, components.Span{Text: text[m[2]:m[3]], Style: th.Markdown.InlineCode})
		last = m[1]
	}
	if last < len(text) {
		out = append(out, highlightPaths(text[last:], th)...)
	}
	if len(out) == 0 {
		out = []components.Span{{Text: text, Style: th.Foreground}}
	}
	return out
}

func highlightPaths(text string, th components.Theme) []components.Span {
	var out []components.Span
	last := 0
	for _, m := range rePathish.FindAllStringIndex(text, -1) {
		tok := text[m[0]:m[1]]
		if !looksHighlightable(tok) {
			continue
		}
		if m[0] > last {
			out = append(out, components.Span{Text: text[last:m[0]], Style: th.Foreground})
		}
		// Paths keep the prose color; the underline is the whole affordance.
		pathSt := th.Foreground
		pathSt.Underline = true
		out = append(out, components.Span{Text: tok, Style: pathSt})
		last = m[1]
	}
	if last < len(text) {
		out = append(out, components.Span{Text: text[last:], Style: th.Foreground})
	}
	if len(out) == 0 && text != "" {
		out = []components.Span{{Text: text, Style: th.Foreground}}
	}
	return out
}

func looksHighlightable(s string) bool {
	if strings.Contains(s, "/") {
		return true
	}
	if strings.Contains(s, ".") {
		return true
	}
	return false
}

// Ensure xui is referenced so goimports doesn't strip it.
var _ = xui.Style{}
