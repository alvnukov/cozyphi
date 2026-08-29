package transcript_test

import (
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func thinkingSnap(state session.State, dur time.Duration) session.Snapshot {
	return session.Snapshot{Messages: []session.Message{{
		ID:               "a1",
		Role:             session.RoleAssistant,
		State:            state,
		ThinkingDuration: dur,
		Content:          []session.ContentBlock{{Type: session.BlockThinking, Text: "pondering"}},
	}}}
}

func requireThinkingBlock(t *testing.T, w components.Widget) *block.ThinkingBlock {
	t.Helper()
	tb, ok := w.(*block.ThinkingBlock)
	if !ok {
		t.Fatalf("widget %T, want *block.ThinkingBlock", w)
	}
	return tb
}

// TestMapperThinkingCollapsedWhileStreaming: reasoning blocks render collapsed
// even mid-stream — the header spinner is the activity signal; expansion
// happens only when the user toggles it.
func TestMapperThinkingCollapsedWhileStreaming(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, thinkingSnap(session.StateStreaming, 0))
	tb := requireThinkingBlock(t, entries[0])
	if tb.Expanded {
		t.Fatal("streaming thinking block must be collapsed by default")
	}
	if !tb.Streaming || tb.Text != "pondering" {
		t.Fatalf("streaming=%v text=%q", tb.Streaming, tb.Text)
	}
}

// TestMapperThinkingToggleSurvivesCompletion: once the user expands a block,
// completion must not collapse it back.
func TestMapperThinkingToggleSurvivesCompletion(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, thinkingSnap(session.StateStreaming, 0))
	tb := requireThinkingBlock(t, entries[0])
	// What Handle runs on Enter/click: flip the widget, then record it.
	tb.Expanded = true
	tb.OnToggle(true)

	entries, _, _ = m.Sync(entries, ids, thinkingSnap(session.StateComplete, 4*time.Second))
	tb = requireThinkingBlock(t, entries[0])
	if !tb.Expanded {
		t.Fatal("user-expanded block collapsed after completion")
	}
	if tb.Duration != 4*time.Second {
		t.Fatalf("duration=%v, want 4s", tb.Duration)
	}
	if tb.Streaming {
		t.Fatal("completed block still streaming")
	}
}

// TestMapperThinkingStaysCollapsedAfterCompletion: a block the user never
// touched stays collapsed when the round ends — the old behavior kept it
// expanded forever with a stale "Thinking" label.
func TestMapperThinkingStaysCollapsedAfterCompletion(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, thinkingSnap(session.StateComplete, 0))
	if tb := requireThinkingBlock(t, entries[0]); tb.Expanded {
		t.Fatal("untouched thinking block rendered expanded")
	}

	entries, _, _ = m.Sync(entries, ids, thinkingSnap(session.StateComplete, 0))
	if tb := requireThinkingBlock(t, entries[0]); tb.Expanded {
		t.Fatal("untouched thinking block expanded on re-sync")
	}
}

// TestMapperThinkingCarriesModelWhileStreaming: the thinking widget receives
// the streaming model so its header can lead with it.
func TestMapperThinkingCarriesModelWhileStreaming(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := thinkingSnap(session.StateStreaming, 0)
	snap.Messages[0].Model = "deepseek-chat"
	entries, _, _ := m.Sync(nil, nil, snap)
	if tb := requireThinkingBlock(t, entries[0]); tb.Model != "deepseek-chat" {
		t.Fatalf("thinking model = %q, want deepseek-chat", tb.Model)
	}
}
