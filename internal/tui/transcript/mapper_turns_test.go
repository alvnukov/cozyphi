package transcript_test

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// turnsSnap builds three finished turns; the first carries thinking, an edit
// and a grep, so it is the one old enough to condense.
func turnsSnap() session.Snapshot {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "one"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Started: base, Ended: base.Add(42 * time.Second),
				Content: []session.ContentBlock{
					{Type: session.BlockThinking, Text: "pondering"},
					{Type: session.BlockToolUse, ID: "t1", Name: "edit", Input: "pane.go"},
					{Type: session.BlockToolUse, ID: "t2", Name: "grep", Input: "pat"},
					{Type: session.BlockText, Text: "done one"},
				},
			},
			{ID: "u2", Role: session.RoleUser, State: session.StateComplete, Text: "two"},
			{
				ID: "a2", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{{Type: session.BlockText, Text: "done two"}},
			},
			{ID: "u3", Role: session.RoleUser, State: session.StateComplete, Text: "three"},
			{
				ID: "a3", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{{Type: session.BlockText, Text: "done three"}},
			},
		},
		Tools: map[string]session.ToolRun{
			"t1": {
				ToolUseID: "t1", Name: "edit", Status: session.ToolDone,
				Detail: "internal/tui/pane.go", Output: mapperDiff,
			},
			"t2": {ToolUseID: "t2", Name: "grep", Status: session.ToolDone, Detail: `"pat" — 3 matches`},
		},
	}
}

func TestOldTurnsCondenseToASummaryRow(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, turnsSnap())
	if len(entries) != 7 {
		t.Fatalf("entries: got %d, want 7 (condensed turn + two full)", len(entries))
	}
	if _, ok := entries[0].(*block.UserBlock); !ok {
		t.Fatalf("the condensed turn keeps its user prompt, got %T", entries[0])
	}
	s, ok := entries[1].(*block.TurnSummaryBlock)
	if !ok {
		t.Fatalf("the old turn's work must fold to a summary, got %T", entries[1])
	}
	if s.Tools != 2 || s.Failed != 0 || s.Duration != 42*time.Second {
		t.Fatalf("summary stats: tools=%d failed=%d dur=%s", s.Tools, s.Failed, s.Duration)
	}
	if len(s.Files) != 1 || s.Files[0] != "pane.go" {
		t.Fatalf("summary files: %v", s.Files)
	}
	if _, ok := entries[2].(*block.AssistantBlock); !ok {
		t.Fatalf("the turn's final reply stays out of the fold, got %T", entries[2])
	}
	// The last two turns render in full: user + reply each.
	for i := 3; i < 7; i += 2 {
		if _, ok := entries[i].(*block.UserBlock); !ok {
			t.Fatalf("entry %d: want user prompt, got %T", i, entries[i])
		}
	}
}

func TestFailedToolStaysVisibleInACondensedTurn(t *testing.T) {
	snap := turnsSnap()
	snap.Tools["t2"] = session.ToolRun{
		ToolUseID: "t2", Name: "grep", Status: session.ToolError, Error: "ripgrep exited with code 2",
	}
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, snap)
	s, ok := entries[1].(*block.TurnSummaryBlock)
	if !ok {
		t.Fatalf("summary row expected, got %T", entries[1])
	}
	if s.Failed != 1 {
		t.Fatalf("summary must count the failure, got %d", s.Failed)
	}
	tool, ok := entries[2].(*block.ToolBlock)
	if !ok {
		t.Fatalf("the failed tool row must stay under the summary, got %T", entries[2])
	}
	if tool.Error == "" {
		t.Fatal("the kept row is the failed one")
	}
}

func TestSummaryToggleUnfoldsAndRefoldsTheTurn(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := turnsSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)
	s := entries[1].(*block.TurnSummaryBlock)

	s.Expanded = true
	s.OnToggle(true)
	entries, ids, _ = m.Sync(entries, ids, snap)
	if len(entries) != 10 {
		t.Fatalf("expanded turn re-emits its rows: got %d entries, want 10", len(entries))
	}
	s = entries[1].(*block.TurnSummaryBlock)
	if !s.Expanded {
		t.Fatal("the summary stays as the fold handle, arrow flipped")
	}
	if _, ok := entries[2].(*block.ThinkingBlock); !ok {
		t.Fatalf("unfolded thinking expected, got %T", entries[2])
	}
	if _, ok := entries[3].(*block.DiffBlock); !ok {
		t.Fatalf("unfolded diff card expected, got %T", entries[3])
	}

	s.Expanded = false
	s.OnToggle(false)
	entries, _, _ = m.Sync(entries, ids, snap)
	if len(entries) != 7 {
		t.Fatalf("refolded turn: got %d entries, want 7", len(entries))
	}
}

func TestVerboseDisablesCondensation(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	m.SetVerbose(true)
	entries, _, _ := m.Sync(nil, nil, turnsSnap())
	if len(entries) != 9 {
		t.Fatalf("verbose renders every row: got %d entries, want 9", len(entries))
	}
	for i, w := range entries {
		if _, ok := w.(*block.TurnSummaryBlock); ok {
			t.Fatalf("entry %d: verbose must not emit summary rows", i)
		}
	}
}

func TestShiftPageKeysJumpBetweenTurns(t *testing.T) {
	p := transcript.NewTranscriptPane(components.DefaultTheme(), nil, "test")
	p.LoadReplay(turnsSnap())
	p.Sync()
	ctx := components.DrawContext{Max: components.Size{Width: 60, Height: 6}}
	p.Draw(ctx, 60, 6)
	if !p.AtBottom() {
		t.Fatal("the transcript starts pinned to the tail")
	}

	ec := &components.EventContext{}
	p.HandlePageKey(ec, xui.KeyEvent{Press: true, Code: xui.KeyPageUp, Mods: xui.ModShift})
	if p.AtBottom() {
		t.Fatal("jumping to the previous turn must leave the tail")
	}
	if !ec.Consume {
		t.Fatal("the turn jump consumes the key")
	}

	// Jumping forward past the last turn re-pins the tail.
	for range 5 {
		p.Draw(ctx, 60, 6)
		ec = &components.EventContext{}
		p.HandlePageKey(ec, xui.KeyEvent{Press: true, Code: xui.KeyPageDown, Mods: xui.ModShift})
	}
	if !p.AtBottom() {
		t.Fatal("jumping past the last turn must re-pin the tail")
	}
}
