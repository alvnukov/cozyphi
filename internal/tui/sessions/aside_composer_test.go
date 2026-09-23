package sessions

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// A message's btw button starts a one-shot question at that message. The
// next normal turn must not receive either the question or its answer.
func TestBtwButtonAsksAtAnchorWithoutPollutingNextTurn(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[1].ID
	require.NotEmpty(t, anchor)

	e.AsideAbout(anchor)
	assert.Contains(t, e.composer.Chat.AgentLabel.Text, "btw @"+anchor)
	e.composer.Chat.Value = "why so?"
	e.composer.Chat.Cursor = len(e.composer.Chat.Value)
	e.composer.Chat.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	waitForAside(t, e)

	got := bodies()
	require.Len(t, got, 3)
	assert.Contains(t, got[2], "reply 1")
	assert.Contains(t, got[2], "why so?")
	assert.NotContains(t, got[2], "reply 2")
	assert.NotContains(t, e.composer.Chat.AgentLabel.Text, "btw", "aside mode is one-shot")

	runTurn(t, e, "third", 7)
	got = bodies()
	require.Len(t, got, 4)
	assert.NotContains(t, got[3], "why so?")
	assert.NotContains(t, got[3], "reply 3")
}

func TestBtwBareChatEnterStartsModeWithoutNormalPrompt(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	e.composer.Chat.Value = "/btw"
	e.composer.Chat.Cursor = len(e.composer.Chat.Value)
	e.composer.Chat.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	e.DrainNow()
	require.Equal(t, "⏵⏵ btw", e.composer.Chat.AgentLabel.Text)
	require.Empty(t, e.composer.Chat.Value)
	require.Empty(t, bodies(), "bare /btw must not publish a prompt")
}
