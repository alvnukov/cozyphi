package block

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
)

// TestAssistantBlockMetaRow: a preformatted end-of-turn metadata row paints
// as one muted line after the answer and stays out of CopyText.
func TestAssistantBlockMetaRow(t *testing.T) {
	th := components.DefaultTheme()
	assistantBlock := &AssistantBlock{
		Text:  "done",
		State: session.StateComplete,
		Meta:  "• deepseek-chat[56k] • 1m 4s",
		Theme: th,
	}
	surface := assistantBlock.Draw(components.DrawContext{
		Max:    components.Size{Width: 60},
		Method: xui.WidthUnicode,
	})
	txt := components.SurfaceText(surface)
	assert.Contains(t, txt, "done")
	assert.Contains(t, txt, "• deepseek-chat[56k] • 1m 4s")
	assert.Equal(t, "done", assistantBlock.CopyText(), "meta stays out of copy text")
	assert.GreaterOrEqual(t, surface.Size.Height, 2, "meta row adds a line")
	assert.True(
		t,
		th.Muted.Equal(surface.Buffer[surface.Size.Width].Style),
		"meta row is muted",
	)
}

// TestAssistantBlockMetaRowOmitted: no meta, no extra line.
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
	assert.NotContains(t, components.SurfaceText(surface), "•")
}
