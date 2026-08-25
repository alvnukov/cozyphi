package block

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestAssistantBlockMetaRow: the end-of-turn row follows opencode's footer —
// a secondary ▣ marker, the model label bright, the remainder muted. It stays
// out of CopyText.
func TestAssistantBlockMetaRow(t *testing.T) {
	th := components.DefaultTheme()
	assistantBlock := &AssistantBlock{
		Text:      "done",
		State:     session.StateComplete,
		MetaLabel: "deepseek-chat[56k]",
		MetaTail:  "1m 4s",
		Theme:     th,
	}
	surface := assistantBlock.Draw(components.DrawContext{
		Max:    components.Size{Width: 60},
		Method: xui.WidthUnicode,
	})
	txt := components.SurfaceText(surface)
	assert.Contains(t, txt, "done")
	assert.Contains(t, txt, "▣ deepseek-chat[56k] · 1m 4s")
	assert.Equal(t, "done", assistantBlock.CopyText(), "meta stays out of copy text")
	assert.GreaterOrEqual(t, surface.Size.Height, 2, "meta row adds a line")

	w := surface.Size.Width
	cell := func(x, y int) xui.Cell { return surface.Buffer[y*w+x] }
	// Row layout at messageIndent: ▣ marker, space, label, " · ", tail.
	assert.Equal(t, "▣", cell(messageIndent, 1).Char, "marker glyph")
	assert.True(t, th.Secondary.Equal(cell(messageIndent, 1).Style), "marker is secondary")
	assert.Equal(t, "d", cell(messageIndent+2, 1).Char, "label starts after marker")
	assert.True(t, th.Foreground.Equal(cell(messageIndent+2, 1).Style), "label is bright")
	tailX := messageIndent + 2 + len("deepseek-chat[56k]")
	assert.True(t, th.Muted.Equal(cell(tailX, 1).Style), "tail after label is muted")
	assert.Equal(t, " ", cell(tailX, 1).Char, "tail opens with a separator space")
}

// TestAssistantBlockMetaRowOmitted: no label, no extra line.
func TestAssistantBlockMetaRowOmitted(t *testing.T) {
	assistantBlock := &AssistantBlock{
		Text:  "done",
		State: session.StateComplete,
		Theme: components.DefaultTheme(),
	}
	surface := assistantBlock.Draw(components.DrawContext{
		Max:    components.Size{Width: 60},
		Method: xui.WidthUnicode,
	})
	assert.Equal(t, 1, surface.Size.Height)
	assert.NotContains(t, components.SurfaceText(surface), "▣")
}
