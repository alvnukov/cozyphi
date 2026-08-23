package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
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

// TestLinesHeadingStyle: heading lines keep the theme's heading style.
func TestLinesHeadingStyle(t *testing.T) {
	th := components.DefaultTheme()
	lines := RenderMarkdownLines("# Hi", th, 40, xui.WidthUnicode)
	require.NotEmpty(t, lines)
	require.NotEmpty(t, lines[0])
	assert.Equal(t, th.Success.Fg, lines[0][0].Style.Fg)
	assert.True(t, lines[0][0].Style.Bold)
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
