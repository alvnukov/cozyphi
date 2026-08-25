package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func lineStrings(lines []components.RichLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		var b strings.Builder
		for _, sp := range l {
			b.WriteString(sp.Text)
		}
		out[i] = b.String()
	}
	return out
}

// TestLinesHangIndentListItems: a wrapped list item's continuation lines
// indent past the marker instead of gluing to the left margin.
func TestLinesHangIndentListItems(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("- alpha beta gamma", th, 12, xui.WidthUnicode)
	assert.Equal(t, []string{"• alpha beta", "  gamma"}, lineStrings(lines))
}

// TestLinesNestedListIndent: nested list markers indent under the parent item.
func TestLinesNestedListIndent(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("- one\n  - two", th, 40, xui.WidthUnicode)
	assert.Equal(t, []string{"• one", "  • two"}, lineStrings(lines))
}

// TestLinesCodeBlockBox: fenced code renders in an opencode-style rounded box
// with the language embedded in the top border (hand-derived worked example:
// content "fmt.Println(1)" is 14 wide → box width 18).
func TestLinesCodeBlockBox(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("```go\nfmt.Println(1)\n```", th, 40, xui.WidthUnicode)
	assert.Equal(t, []string{
		"╭─ go ───────────╮",
		"│ fmt.Println(1) │",
		"╰────────────────╯",
	}, lineStrings(lines))
}

// TestLinesCodeBoxShrinksToFitTerminal: when the terminal is narrower than the
// code, the box clamps to the available width and the code wraps inside.
func TestLinesCodeBoxShrinksToFitTerminal(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("```\naaaaaaaaaa\n```", th, 8, xui.WidthUnicode)
	assert.Equal(t, []string{
		"╭──────╮",
		"│ aaaa │",
		"│ aaaa │",
		"│ aa   │",
		"╰──────╯",
	}, lineStrings(lines))
}

// TestLinesBlockquoteRuleOnEveryWrappedLine: the quote rule prefixes each
// wrapped line, not just the first source line.
func TestLinesBlockquoteRuleOnEveryWrappedLine(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("> alpha beta gamma", th, 12, xui.WidthUnicode)
	assert.Equal(t, []string{"▎ alpha beta", "▎ gamma"}, lineStrings(lines))
}

// TestLinesParagraphRhythm: blank lines between blocks survive wrapping.
func TestLinesParagraphRhythm(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("a\n\nb", th, 40, xui.WidthUnicode)
	assert.Equal(t, []string{"a", "", "b"}, lineStrings(lines))
}

// TestLinesHeadingStyle: heading lines keep the theme's heading role style.
func TestLinesHeadingStyle(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("# Hi", th, 40, xui.WidthUnicode)
	require.NotEmpty(t, lines)
	require.NotEmpty(t, lines[0])
	assert.Equal(t, th.Markdown.Heading.Fg, lines[0][0].Style.Fg)
	assert.True(t, lines[0][0].Style.Bold)
}

// findSpan returns the first span whose trimmed text equals want.
func findSpan(t *testing.T, lines []components.RichLine, want string) components.Span {
	t.Helper()
	for _, l := range lines {
		for _, sp := range l {
			if strings.TrimSpace(sp.Text) == want {
				return sp
			}
		}
	}
	t.Fatalf("span %q not found in %q", want, lineStrings(lines))
	return components.Span{}
}

// TestLinesOpencodeProseStyles pins prose rendering to the real opencode.json
// roles: H1 purple bold underlined, deeper headings the same purple bold,
// strong orange, emphasis yellow italic, inline code green, link labels cyan
// underlined, bullets peach, ordered markers cyan.
func TestLinesOpencodeProseStyles(t *testing.T) {
	th := components.OpencodeTheme()
	lines := RenderMarkdownLines(
		"# Title\n\n## Sub\n\n**strong** and *emph* and `code` and [label](https://x)\n\n- item\n\n1. one",
		th, 80, xui.WidthUnicode,
	)
	purple := xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8), Bold: true}
	h1 := purple
	h1.Underline = true
	assert.Equal(t, h1, findSpan(t, lines, "Title").Style)
	assert.Equal(t, purple, findSpan(t, lines, "Sub").Style)
	strong := xui.Style{Fg: xui.RGBColor(0xf5, 0xa7, 0x42), Bold: true}
	assert.Equal(t, strong, findSpan(t, lines, "strong").Style)
	emph := xui.Style{Fg: xui.RGBColor(0xe5, 0xc0, 0x7b), Italic: true}
	assert.Equal(t, emph, findSpan(t, lines, "emph").Style)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)}, findSpan(t, lines, "code").Style)
	link := xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2), Underline: true}
	assert.Equal(t, link, findSpan(t, lines, "label").Style)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xfa, 0xb2, 0x83)}, findSpan(t, lines, "•").Style)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x56, 0xb6, 0xc2)}, findSpan(t, lines, "1.").Style)
}

// TestLinesOpencodeCodeBoxStyles: highlighted Go follows the opencode syntax
// palette (purple keyword, green string); a no-language block renders in the
// plain code-block color, not warning orange.
func TestLinesOpencodeCodeBoxStyles(t *testing.T) {
	th := components.OpencodeTheme()
	lines := RenderMarkdownLines("```go\nvar s = \"x\"\n```", th, 40, xui.WidthUnicode)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x9d, 0x7c, 0xd8)}, findSpan(t, lines, "var").Style)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0x7f, 0xd8, 0x8f)}, findSpan(t, lines, `"x"`).Style)
	plain := RenderMarkdownLines("```\nplain\n```", th, 40, xui.WidthUnicode)
	assert.Equal(t, xui.Style{Fg: xui.RGBColor(0xee, 0xee, 0xee)}, findSpan(t, plain, "plain").Style)
}

// TestLinesEmpty: empty input renders nothing (caller owns the placeholder).
func TestLinesEmpty(t *testing.T) {
	assert.Nil(t, RenderMarkdownLines("", components.DefaultTheme(), 40, xui.WidthUnicode))
}

// TestLinesPreview prints a realistic answer for eyeballing; run with -v.
func TestLinesPreview(t *testing.T) {
	src := "## Что хорошо\n\n- **Инварианта в одном месте.** Executor.runOne — единственная точка,\n  где соблюдается Pre → Gate → Run → Post, обойти gate изнутри нельзя.\n- Seam'ы настоящие: `Gate = чистый Check + колбэк Ask`, минимум два адаптера.\n\n```go\nfunc (e *Engine) Loop(ctx context.Context) error {\n\treturn e.executor.Run(ctx, msg)\n}\n```\n\n> Движок отдаёт iter.Seq2, контроллер редуцирует это в Msg на шину.\n\n1. первый пункт списка с достаточно длинным текстом для переноса\n2. второй пункт\n"
	lines := RenderMarkdownLines(src, components.DefaultTheme(), 72, xui.WidthUnicode)
	var b strings.Builder
	for _, l := range lines {
		var lb strings.Builder
		for _, sp := range l {
			lb.WriteString(sp.Text)
		}
		b.WriteString(lb.String() + "\n")
	}
	t.Log("\n" + b.String())
}
