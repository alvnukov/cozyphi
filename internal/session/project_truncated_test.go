package session

import (
	"strings"
	"testing"
	"time"
)

// TestProjectMaxTokensWithoutTextShowsWarning: the observed failure — a round
// that spent its whole output budget on reasoning ends with no answer text.
// The transcript must still say what happened instead of rendering nothing.
func TestProjectMaxTokensWithoutTextShowsWarning(t *testing.T) {
	started := time.Date(2026, 8, 24, 14, 47, 0, 0, time.UTC)
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete,
		StopReason: StopMaxTokens,
		Model:      "deepseek-v4-pro", Started: started, Ended: started.Add(86 * time.Second),
		Usage:   TokenUsage{PromptTokens: 73616, CompletionTokens: 4096, TotalTokens: 77712},
		Content: []ContentBlock{{Type: BlockThinking, Text: "long reasoning"}},
	}}}
	items := Project(s)
	if len(items) == 0 || items[0].Kind != ItemThinking {
		t.Fatalf("thinking row must remain: %+v", items)
	}
	var warning *Item
	for i := range items {
		if items[i].Kind == ItemAssistant {
			warning = &items[i]
		}
	}
	if warning == nil {
		t.Fatalf("no warning row for a truncated round: %+v", items)
	}
	if !strings.Contains(warning.Text, "max_output_tokens") {
		t.Fatalf("warning must name the setting: %q", warning.Text)
	}
	if !warning.TurnMeta.Truncated {
		t.Fatalf("warning row meta must be flagged truncated: %+v", warning.TurnMeta)
	}
	if warning.TurnMeta.Model != "deepseek-v4-pro" {
		t.Fatalf("warning row keeps the turn footer: %+v", warning.TurnMeta)
	}
}

// TestProjectMaxTokensWithTextFlagsMeta: a round truncated mid-answer keeps
// its text; the footer reports the truncation instead of adding a row.
func TestProjectMaxTokensWithTextFlagsMeta(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete, StopReason: StopMaxTokens,
		Model:   "m",
		Content: []ContentBlock{{Type: BlockText, Text: "partial answer"}},
	}}}
	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemAssistant || items[0].Text != "partial answer" {
		t.Fatalf("items: %+v", items)
	}
	if !items[0].TurnMeta.Truncated {
		t.Fatalf("meta must be flagged truncated: %+v", items[0].TurnMeta)
	}
}

// TestProjectEndTurnNotFlagged: ordinary rounds never claim truncation.
func TestProjectEndTurnNotFlagged(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete, StopReason: StopEndTurn,
		Model:   "m",
		Content: []ContentBlock{{Type: BlockText, Text: "answer"}},
	}}}
	items := Project(s)
	if len(items) != 1 || items[0].TurnMeta.Truncated {
		t.Fatalf("items: %+v", items)
	}
}
