package tui_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui"
)

func TestMapperSyncDirtyOnlyChangedRows(t *testing.T) {
	m := tui.NewMapper(components.DefaultTheme(), nil, nil)
	snap := session.Snapshot{
		Messages: []session.Message{
			{
				ID:    "u1",
				Role:  session.RoleUser,
				State: session.StateComplete,
				Text:  "hello",
			},
			{
				ID:    "a1",
				Role:  session.RoleAssistant,
				State: session.StateStreaming,
				Content: []session.ContentBlock{
					{Type: session.BlockText, Text: "hi"},
				},
			},
		},
	}
	entries, ids, dirty := m.Sync(nil, nil, snap)
	if len(dirty) != 2 {
		t.Fatalf("initial dirty=%v", dirty)
	}

	// Stream more assistant text — only assistant row should be dirty.
	snap.Messages[1].Content[0].Text = "hi there"
	entries, ids, dirty = m.Sync(entries, ids, snap)
	if len(dirty) != 1 || dirty[0] != 1 {
		t.Fatalf("stream dirty=%v want [1]", dirty)
	}
	ab, ok := entries[1].(*block.AssistantBlock)
	if !ok || ab.Text != "hi there" {
		t.Fatalf("assistant %+v", entries[1])
	}

	// No-op sync — nothing dirty.
	entries, ids, dirty = m.Sync(entries, ids, snap)
	if len(dirty) != 0 {
		t.Fatalf("noop dirty=%v", dirty)
	}

	// Append a new user message — only the new index dirty.
	snap.Messages = append(snap.Messages, session.Message{
		ID:    "u2",
		Role:  session.RoleUser,
		State: session.StateComplete,
		Text:  "more",
	})
	_, _, dirty = m.Sync(entries, ids, snap)
	if len(dirty) != 1 || dirty[0] != 2 {
		t.Fatalf("append dirty=%v want [2]", dirty)
	}
}

func TestApplyProgressReportsChange(t *testing.T) {
	s := tui.NewSubagentStore()
	s.Bind("job1", "parent1")
	p := job.Progress{
		JobID:           "job1",
		ParentToolUseID: "parent1",
		ToolUseID:       "c1",
		Name:            "read",
		Status:          "in-progress",
		Detail:          "a.go",
	}
	if !s.ApplyProgress(p) {
		t.Fatal("first apply should change")
	}
	if s.ApplyProgress(p) {
		t.Fatal("identical apply should not change")
	}
	p.Status = "done"
	if !s.ApplyProgress(p) {
		t.Fatal("status change should dirty")
	}
}
