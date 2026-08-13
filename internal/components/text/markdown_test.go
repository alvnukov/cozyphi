package text

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
)

func TestRenderMarkdown_Basics(t *testing.T) {
	th := components.DefaultTheme()

	tests := []struct {
		name         string
		src          string
		wantContains []string
		wantAbsent   []string
		check        func(t *testing.T, spans []components.Span)
	}{
		{
			name:         "strips heading marker",
			src:          "# Hello",
			wantContains: []string{"Hello"},
			wantAbsent:   []string{"#"},
			check: func(t *testing.T, spans []components.Span) {
				require.NotEmpty(t, spans)
				assert.True(t, spans[0].Style.Bold)
				assert.Equal(t, th.Success.Fg, spans[0].Style.Fg)
			},
		},
		{
			name:         "inline code strips backticks",
			src:          "run `go test` now",
			wantContains: []string{"run ", "go test", " now"},
			wantAbsent:   []string{"`"},
			check: func(t *testing.T, spans []components.Span) {
				found := false
				for _, s := range spans {
					if s.Text == "go test" {
						found = true
						assert.True(t, s.Style.Equal(th.Warning))
					}
				}
				assert.True(t, found, "expected warning-styled code span")
			},
		},
		{
			name:         "emphasis",
			src:          "say **bold** and *italic*",
			wantContains: []string{"bold", "italic"},
			wantAbsent:   []string{"*", "**"},
			check: func(t *testing.T, spans []components.Span) {
				var sawBold, sawItalic bool
				for _, s := range spans {
					if s.Text == "bold" && s.Style.Bold {
						sawBold = true
					}
					if s.Text == "italic" && s.Style.Italic {
						sawItalic = true
					}
				}
				assert.True(t, sawBold && sawItalic)
			},
		},
		{
			name:         "unordered list bullets",
			src:          "- alpha\n- beta",
			wantContains: []string{"• ", "alpha", "beta"},
			wantAbsent:   []string{"- alpha"},
		},
		{
			name:         "fenced code frame",
			src:          "```go\nfmt.Println(1)\n```",
			wantContains: []string{"╭ go", "│ ", "fmt.Println", "╰"},
			wantAbsent:   []string{"```"},
		},
		{
			name:         "preserves soft newlines",
			src:          "line one\nline two",
			wantContains: []string{"line one\nline two"},
		},
		{
			name:         "link accent",
			src:          "see [docs](https://example.com)",
			wantContains: []string{"docs"},
			wantAbsent:   []string{"https://example.com", "[", "]"},
			check: func(t *testing.T, spans []components.Span) {
				for _, s := range spans {
					if s.Text == "docs" {
						assert.Equal(t, th.Accent.Fg, s.Style.Fg)
						return
					}
				}
				t.Fatal("docs span not found")
			},
		},
		{
			name:         "blockquote rule",
			src:          "> quoted",
			wantContains: []string{"▎ ", "quoted"},
			wantAbsent:   []string{">"},
		},
		{
			name:         "path highlight in prose",
			src:          "open internal/components/block.go please",
			wantContains: []string{"internal/components/block.go"},
			check: func(t *testing.T, spans []components.Span) {
				for _, s := range spans {
					if strings.Contains(s.Text, "internal/") {
						assert.True(t, s.Style.Equal(th.Warning), "path style")
						return
					}
				}
				t.Fatal("path span not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := RenderMarkdown(tt.src, th)
			got := spansText(spans)
			for _, w := range tt.wantContains {
				assert.Contains(t, got, w)
			}
			for _, a := range tt.wantAbsent {
				assert.NotContains(t, got, a)
			}
			if tt.check != nil {
				tt.check(t, spans)
			}
		})
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	assert.Nil(t, RenderMarkdown("", components.DefaultTheme()))
}
