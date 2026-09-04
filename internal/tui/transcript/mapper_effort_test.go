package transcript_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// TestMapperThinkingHeaderShowsEffort: the streaming header names the
// model the way every info line does — "model · effort" while one is set.
func TestMapperThinkingHeaderShowsEffort(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := thinkingSnap(session.StateStreaming, 0)
	snap.Messages[0].Model = "deepseek-chat"
	snap.Messages[0].Effort = "high"

	entries, _, _ := m.Sync(nil, nil, snap)
	tb := requireThinkingBlock(t, entries[0])
	require.Equal(t, "deepseek-chat · high", tb.Model)
}

// TestMapperThinkingHeaderWithoutEffortStaysBare: no effort, no suffix —
// the header keeps rendering the plain model name.
func TestMapperThinkingHeaderWithoutEffortStaysBare(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := thinkingSnap(session.StateStreaming, 0)
	snap.Messages[0].Model = "deepseek-chat"

	entries, _, _ := m.Sync(nil, nil, snap)
	tb := requireThinkingBlock(t, entries[0])
	require.Equal(t, "deepseek-chat", tb.Model)
}

// TestMapperTurnMetaLabelShowsEffort: the end-of-turn row shows the same
// label, so the streaming header and the settled row never disagree.
func TestMapperTurnMetaLabelShowsEffort(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := session.Snapshot{Messages: []session.Message{{
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Model: "m", Effort: "high",
		Started:    time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		Ended:      time.Date(2026, 8, 23, 12, 0, 4, 0, time.UTC),
		Content:    []session.ContentBlock{{Type: session.BlockText, Text: "answer"}},
		StopReason: session.StopEndTurn,
	}}}

	entries, _, _ := m.Sync(nil, nil, snap)
	ab, ok := entries[0].(*block.AssistantBlock)
	require.True(t, ok, "entries[0] = %+v", entries[0])
	require.Equal(t, "m · high", ab.MetaLabel)
	require.Equal(t, "4s", ab.MetaTail)
}
