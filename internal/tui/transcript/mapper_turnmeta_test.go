package transcript_test

import (
	"testing"
	"time"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

// TestMapperFormatsTurnMeta: a terminal round's tail row carries the
// opencode-style footer parts (bright model label, muted duration tail); a
// streaming round carries none, and completing the turn dirties the row.
func TestMapperFormatsTurnMeta(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	started := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	streaming := session.Snapshot{Messages: []session.Message{{
		ID: "a1", Role: session.RoleAssistant, State: session.StateStreaming,
		Model: "deepseek-chat", Started: started,
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "partial"}},
	}}}
	entries, ids, _ := m.Sync(nil, nil, streaming)
	ab, ok := entries[0].(*block.AssistantBlock)
	if !ok || ab.MetaLabel != "" {
		t.Fatalf("streaming meta=%q entries=%+v", ab.MetaLabel, entries)
	}

	complete := session.Snapshot{Messages: []session.Message{{
		ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
		Model: "deepseek-chat", Started: started, Ended: started.Add(64 * time.Second),
		Usage:   session.TokenUsage{PromptTokens: 56000, TotalTokens: 57000},
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "final answer"}},
	}}}
	entries, _, dirty := m.Sync(entries, ids, complete)
	ab, ok = entries[0].(*block.AssistantBlock)
	if !ok {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	if ab.MetaLabel != "deepseek-chat[56k]" || ab.MetaTail != "1m 4s" {
		t.Fatalf("meta = %q / %q", ab.MetaLabel, ab.MetaTail)
	}
	if len(dirty) != 1 {
		t.Fatalf("completing the turn should dirty the row, dirty=%v", dirty)
	}
}

// TestMapperTurnMetaVariants: duration formats follow opencode (4s, 1m 4s,
// 1h 2m); unknown usage drops the context bracket.
func TestMapperTurnMetaVariants(t *testing.T) {
	cases := []struct {
		name      string
		meta      session.TurnMeta
		wantLabel string
		wantTail  string
	}{
		{"seconds", session.TurnMeta{Model: "m", Duration: 4 * time.Second}, "m", "4s"},
		{"minutes", session.TurnMeta{Model: "m", Duration: 64 * time.Second}, "m", "1m 4s"},
		{"hours", session.TurnMeta{Model: "m", Duration: 3720 * time.Second}, "m", "1h 2m"},
		{"context bracket", session.TurnMeta{
			Model: "m", Duration: 4 * time.Second,
			Usage: session.TokenUsage{PromptTokens: 1200, TotalTokens: 2000},
		}, "m[1.2k]", "4s"},
		{"no model hides row", session.TurnMeta{Duration: 4 * time.Second}, "", ""},
		{"model only", session.TurnMeta{Model: "m"}, "m", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
			snap := session.Snapshot{Messages: []session.Message{{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Model:   tc.meta.Model,
				Started: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
				Ended:   time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC).Add(tc.meta.Duration),
				Usage:   tc.meta.Usage,
				Content: []session.ContentBlock{{Type: session.BlockText, Text: "answer"}},
			}}}
			entries, _, _ := m.Sync(nil, nil, snap)
			ab, ok := entries[0].(*block.AssistantBlock)
			if !ok {
				t.Fatalf("entries[0] = %+v", entries[0])
			}
			if ab.MetaLabel != tc.wantLabel || ab.MetaTail != tc.wantTail {
				t.Fatalf("meta = %q / %q, want %q / %q", ab.MetaLabel, ab.MetaTail, tc.wantLabel, tc.wantTail)
			}
		})
	}
}
