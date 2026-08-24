package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

func wiredPane(t *testing.T) (*ComposerPane, *fakeBus) {
	t.Helper()
	c := newTestPane()
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "/tmp", bus, &fakeFocus{})
	return c, bus
}

func pressTab(c *ComposerPane) *components.EventContext {
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyTab})
	return ctx
}

// TestComposerTabTogglesMode: Tab with no completer open publishes
// ModeToggleMsg and consumes the event.
func TestComposerTabTogglesMode(t *testing.T) {
	c, bus := wiredPane(t)

	ctx := pressTab(c)

	require.Equal(t, controller.ModeToggleMsg{}, bus.published)
	require.True(t, bus.drained)
	require.True(t, ctx.Consume)
}

// TestComposerTabFeedsOpenPicker: while a picker is open, Tab belongs to the
// picker, not the mode toggle.
func TestComposerTabFeedsOpenPicker(t *testing.T) {
	c, bus := wiredPane(t)
	c.mention.Show()
	c.Chat.MentionOpen = true

	pressTab(c)

	require.Nil(t, bus.published)
}

// TestComposerSetModeLabel pins the opencode-style posture label in the
// composer meta row.
func TestComposerSetModeLabel(t *testing.T) {
	c, _ := wiredPane(t)

	c.SetMode(false)
	require.Equal(t, "⏵⏵ build", c.Chat.AgentLabel.Text)
	require.True(t, c.Chat.AgentLabel.Style.Equal(c.theme.Secondary))

	c.SetMode(true)
	require.Equal(t, "⏵⏵ plan", c.Chat.AgentLabel.Text)
	require.True(t, c.Chat.AgentLabel.Style.Equal(c.theme.Warning))
}

// TestComposerMentionOffersAgents: the @ picker lists sub-agent roles that
// match the query, ahead of any file results, and keeps them after the async
// file search replaces the list.
func TestComposerMentionOffersAgents(t *testing.T) {
	c, _ := wiredPane(t)

	c.onMentionChange(true, "ex")
	require.True(t, c.mention.Open)
	require.NotEmpty(t, c.mention.Items)
	require.Equal(t, "explore", c.mention.Items[0].Path)
	require.True(t, c.mention.Items[0].Agent)

	// File results arrive async: agents must survive the merge.
	c.ApplyMentionResults(controller.MentionResultsMsg{
		Gen:   c.mentionGen,
		Query: "ex",
		Paths: []string{"examples/main.go"},
	})
	require.Equal(t, "explore", c.mention.Items[0].Path)
	require.Len(t, c.mention.Items, 2)
	require.Equal(t, "examples/main.go", c.mention.Items[1].Path)

	// No role matches this query: file results only.
	c.onMentionChange(true, "zzz")
	for _, it := range c.mention.Items {
		require.False(t, it.Agent)
	}
}

// TestComposerAcceptAgentMention: accepting a role inserts "@role " exactly
// like a file mention, so the engine's delegation parser sees it.
func TestComposerAcceptAgentMention(t *testing.T) {
	c, _ := wiredPane(t)
	c.Chat.Value = "@ex"
	c.Chat.Cursor = len(c.Chat.Value)

	c.onMentionChange(true, "ex")
	c.acceptMention(c.mention.Items[0])

	require.Equal(t, "@explore ", c.Chat.Value)
}
