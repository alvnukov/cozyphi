package block

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
)

func BenchmarkAssistantBlockDrawStreaming(b *testing.B) {
	for _, paragraphs := range []int{50, 200, 400} {
		b.Run(fmt.Sprintf("paragraphs_%d", paragraphs), func(b *testing.B) {
			answer := strings.Repeat(
				"A growing streaming answer with `code` and internal/path/file.go.\n\n",
				paragraphs,
			)
			block := AssistantBlock{
				Text:  answer,
				State: session.StateStreaming,
				Theme: components.DefaultTheme(),
			}
			ctx := components.DrawContext{
				Max:    components.Size{Width: 100, Height: 10_000},
				Method: xui.WidthUnicode,
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = block.Draw(ctx)
			}
		})
	}
}
