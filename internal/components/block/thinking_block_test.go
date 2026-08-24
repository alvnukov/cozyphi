package block

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

func drawThinking(t *testing.T, b *ThinkingBlock) components.Surface {
	t.Helper()
	return b.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 20},
		Method: xui.WidthUnicode,
	})
}

func thinkingHeader(s components.Surface) string {
	return components.ExtractSurfaceText(s, 0, 0, s.Size.Width-1, 0)
}

// TestThinkingBlockHeaderStates pins the header label: "Thinking" while
// streaming (the spinner animates via the shared scheduler wake) or
// interrupted, "Thought" once done, with the wall-clock span appended
// opencode-style once it is at least a second.
func TestThinkingBlockHeaderStates(t *testing.T) {
	cases := []struct {
		name    string
		block   ThinkingBlock
		want    string
		notWant string
	}{
		{
			name:    "streaming",
			block:   ThinkingBlock{Text: "deliberating", Streaming: true},
			want:    "Thinking",
			notWant: "Thought",
		},
		{
			name:  "done untimed",
			block: ThinkingBlock{Text: "done"},
			want:  "Thought",
		},
		{
			name:    "done under a second",
			block:   ThinkingBlock{Text: "done", Duration: 900 * time.Millisecond},
			want:    "Thought",
			notWant: "for",
		},
		{
			name:  "done for 4s",
			block: ThinkingBlock{Text: "done", Duration: 4 * time.Second},
			want:  "Thought for 4s",
		},
		{
			name:  "done for 1m 34s",
			block: ThinkingBlock{Text: "done", Duration: 94 * time.Second},
			want:  "Thought for 1m 34s",
		},
		{
			name:  "interrupted",
			block: ThinkingBlock{Text: "cut off", Interrupted: true},
			want:  "Thinking (interrupted)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.block
			b.Theme = components.DefaultTheme()
			got := thinkingHeader(drawThinking(t, &b))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("header %q does not contain %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Fatalf("header %q must not contain %q", got, tc.notWant)
			}
		})
	}
}

// TestThinkingBlockCollapsedByDefault: reasoning renders as a single header
// row unless expanded — streaming included; expansion is the only thing that
// reveals the body.
func TestThinkingBlockCollapsedByDefault(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		b := &ThinkingBlock{
			Text:      strings.Repeat("ponder ", 40),
			Streaming: streaming,
			Theme:     components.DefaultTheme(),
		}
		s := drawThinking(t, b)
		if s.Size.Height != 1 {
			t.Fatalf("streaming=%v collapsed height=%d, want 1", streaming, s.Size.Height)
		}
	}
	b := &ThinkingBlock{Text: "body", Expanded: true, Theme: components.DefaultTheme()}
	if s := drawThinking(t, b); s.Size.Height < 2 {
		t.Fatalf("expanded height=%d, want header + body", s.Size.Height)
	}
}

// TestThinkingBlockBodyRendersMarkdown: the reasoning body uses the shared
// Markdown renderer — markers are stripped and inline constructs keep their
// themed styles instead of the old flat muted-dim span.
func TestThinkingBlockBodyRendersMarkdown(t *testing.T) {
	th := components.DefaultTheme()
	b := &ThinkingBlock{
		Text:     "weigh **two** options and run `go test`",
		Expanded: true,
		Theme:    th,
	}
	s := drawThinking(t, b)

	got := components.SurfaceText(s)
	if !strings.Contains(got, "weigh two options and run go test") {
		t.Fatalf("body = %q, want markdown text with markers stripped", got)
	}
	if strings.Contains(got, "**") || strings.Contains(got, "`") {
		t.Fatalf("body = %q, still contains markdown markers", got)
	}
	if !th.Foreground.Equal(s.Buffer[s.Size.Width+messageIndent].Style) {
		t.Fatalf("body lead style = %+v, want foreground", s.Buffer[s.Size.Width+messageIndent].Style)
	}

	var sawStrong, sawInlineCode bool
	for y := 1; y < s.Size.Height; y++ {
		for x := 0; x < s.Size.Width; x++ {
			style := s.Buffer[y*s.Size.Width+x].Style
			sawStrong = sawStrong || style.Equal(th.Markdown.Strong)
			sawInlineCode = sawInlineCode || style.Equal(th.Markdown.InlineCode)
		}
	}
	if !sawStrong || !sawInlineCode {
		t.Fatalf("body styles strong=%v inlineCode=%v, want both", sawStrong, sawInlineCode)
	}
}
