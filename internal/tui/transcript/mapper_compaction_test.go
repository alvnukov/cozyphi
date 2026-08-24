package transcript_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

func compactionSnap() session.Snapshot {
	return session.Snapshot{Messages: []session.Message{{
		ID:      "c1",
		Role:    session.RoleCompaction,
		Text:    "Compacted 12 messages · 56k → ~8k context · 4 kept",
		Summary: "the user built a slot module and merged it",
	}}}
}

func requireCompactionBlock(t *testing.T, w components.Widget) *block.CompactionBlock {
	t.Helper()
	cb, ok := w.(*block.CompactionBlock)
	if !ok {
		t.Fatalf("widget %T, want *block.CompactionBlock", w)
	}
	return cb
}

// TestMapperCompactionCarriesSummary: the compaction row keeps its report as
// the rule label and carries the summarize body so the row can expand.
func TestMapperCompactionCarriesSummary(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, compactionSnap())
	cb := requireCompactionBlock(t, entries[0])
	if !strings.Contains(cb.Text, "Compacted 12 messages") {
		t.Fatalf("text = %q", cb.Text)
	}
	if cb.Summary == "" {
		t.Fatal("summary dropped by mapper")
	}
	if cb.Expanded {
		t.Fatal("compaction row must start collapsed")
	}
	if cb.OnToggle == nil {
		t.Fatal("OnToggle not wired")
	}
}

// TestMapperCompactionToggleSurvivesResync: like thinking blocks, a row the
// user expanded stays expanded across snapshot syncs.
func TestMapperCompactionToggleSurvivesResync(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, compactionSnap())
	cb := requireCompactionBlock(t, entries[0])
	// What Handle runs on Enter/click: flip the widget, then record it.
	cb.Expanded = true
	cb.OnToggle(true)

	entries, _, _ = m.Sync(entries, ids, compactionSnap())
	cb = requireCompactionBlock(t, entries[0])
	if !cb.Expanded {
		t.Fatal("user-expanded compaction row collapsed on resync")
	}
}
