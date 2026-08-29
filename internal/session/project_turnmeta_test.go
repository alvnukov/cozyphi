package session

import (
	"testing"
	"time"
)

// TestProjectTurnMetaOnTerminalTail: a completed round carries model,
// duration, and usage on its tail text row only — text before a tool call
// stays clean so the metadata reads as end-of-turn, not per paragraph.
func TestProjectTurnMetaOnTerminalTail(t *testing.T) {
	started := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	ended := started.Add(64 * time.Second)
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete,
		Model: "deepseek-chat", Started: started, Ended: ended,
		Usage: TokenUsage{PromptTokens: 1200, TotalTokens: 2000},
		Content: []ContentBlock{
			{Type: BlockText, Text: "before tools"},
			{Type: BlockToolUse, ID: "t1", Name: "bash"},
			{Type: BlockText, Text: "final answer"},
		},
	}}}
	items := Project(s)
	var first, last Item
	for _, it := range items {
		if it.Kind != ItemAssistant {
			continue
		}
		if first.ID == "" {
			first = it
		}
		last = it
	}
	if first.ID == "" || first.Text != "before tools" || last.Text != "final answer" {
		t.Fatalf("assistant items: %+v", items)
	}
	if first.TurnMeta != (TurnMeta{}) {
		t.Fatalf("mid-round text carries meta: %+v", first.TurnMeta)
	}
	want := TurnMeta{
		Model:    "deepseek-chat",
		Duration: 64 * time.Second,
		Usage:    TokenUsage{PromptTokens: 1200, TotalTokens: 2000},
	}
	if last.TurnMeta != want {
		t.Fatalf("tail meta = %+v, want %+v", last.TurnMeta, want)
	}
}

// TestProjectTurnMetaHiddenWhileStreaming: a still-streaming round shows a
// spinner, not a duration — the meta row appears only on terminal states.
func TestProjectTurnMetaHiddenWhileStreaming(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateStreaming,
		Model: "m", Started: time.Now(),
		Content: []ContentBlock{{Type: BlockText, Text: "partial"}},
	}}}
	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemAssistant {
		t.Fatalf("items: %+v", items)
	}
	if items[0].TurnMeta != (TurnMeta{}) {
		t.Fatalf("streaming item carries meta: %+v", items[0].TurnMeta)
	}
}

// TestProjectTurnMetaNeedsTiming: a terminal round without timestamps (e.g.
// replayed history) still shows the model, with no duration.
func TestProjectTurnMetaNeedsTiming(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete,
		Model:   "m",
		Content: []ContentBlock{{Type: BlockText, Text: "answer"}},
	}}}
	items := Project(s)
	if len(items) != 1 || items[0].TurnMeta.Model != "m" || items[0].TurnMeta.Duration != 0 {
		t.Fatalf("items: %+v", items)
	}
}
