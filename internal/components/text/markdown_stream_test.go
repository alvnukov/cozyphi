package text

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/components"
)

func TestMarkdownStreamMatchesFullRenderer(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "paragraphs", chunks: []string{"first", " paragraph\n\n", "second", " paragraph\n\n", "tail"}},
		{name: "list continuation", chunks: []string{"intro\n\n", "- one", "\n\n- two", "\n  continued", "\n\noutro"}},
		{
			name:   "fenced code",
			chunks: []string{"intro\n\n```go\n", "fmt.Println(1)", "\n\nfmt.Println(2)", "\n```\n\n", "tail"},
		},
		{name: "plain fence", chunks: []string{"intro\n\n```\n", "one", "\n\ntwo", "\n```\n\n", "tail"}},
		{name: "blockquote", chunks: []string{"intro\n\n", "> alpha", "\n> beta", "\n\ntail"}},
		{name: "table", chunks: []string{"intro\n\n", "| a |\n", "| - |\n", "| b |", "\n\ntail"}},
		{name: "reference link", chunks: []string{"[docs][ref]", "\n\n[ref]: https://example.com", "\n\ntail"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stream MarkdownStream
			var source strings.Builder
			for _, chunk := range tt.chunks {
				source.WriteString(chunk)
				src := source.String()
				got := stream.Render(src, components.DefaultTheme(), 40, xui.WidthUnicode)
				want := RenderMarkdownLines(src, components.DefaultTheme(), 40, xui.WidthUnicode)
				assert.Equal(t, want, got, "source %q", src)
			}
		})
	}
}

func TestMarkdownStreamResetsOnPrefixEdit(t *testing.T) {
	var stream MarkdownStream
	theme := components.DefaultTheme()
	_ = stream.Render("first\n\nsecond", theme, 40, xui.WidthUnicode)

	got := stream.Render("changed\n\nsecond", theme, 40, xui.WidthUnicode)
	want := RenderMarkdownLines("changed\n\nsecond", theme, 40, xui.WidthUnicode)
	assert.Equal(t, want, got)
}

func TestMarkdownStreamMatchesFullRendererByteByByte(t *testing.T) {
	source := "# Heading\n\nParagraph with **bold**.\n\n" +
		"- first item\n- second item\n\n" +
		"> quoted line\n> continuation\n\n" +
		"```\nplain code\n\nmore code\n```\n\n" +
		"[docs][ref]\n\n[ref]: https://example.com\n"
	var stream MarkdownStream
	for end := 1; end <= len(source); end++ {
		prefix := source[:end]
		got := stream.Render(prefix, components.DefaultTheme(), 32, xui.WidthUnicode)
		want := RenderMarkdownLines(prefix, components.DefaultTheme(), 32, xui.WidthUnicode)
		assert.Equal(t, want, got, "prefix length %d: %q", end, prefix)
	}
}
