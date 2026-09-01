package transcript_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

const mapperDiff = "--- a/pane.go\n+++ b/pane.go\n@@ -1,2 +1,2 @@\n-old\n+new\n"

func diffSnap() session.Snapshot {
	return session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "fix it"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{
					{Type: session.BlockToolUse, ID: "t1", Name: "edit", Input: "pane.go"},
				},
			},
		},
		Tools: map[string]session.ToolRun{
			"t1": {
				ToolUseID: "t1", Name: "edit", Status: session.ToolDone,
				Detail: "pane.go", Output: mapperDiff,
			},
		},
	}
}

func TestEditRoutesToDiffCardOpenWhileTheTurnRuns(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := diffSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)
	if len(entries) != 2 {
		t.Fatalf("entries: %d", len(entries))
	}
	d, ok := entries[1].(*block.DiffBlock)
	if !ok {
		t.Fatalf("edit must render a diff card, got %T", entries[1])
	}
	if d.Path != "pane.go" || d.Diff != mapperDiff {
		t.Fatalf("card fields: path=%q diff=%q", d.Path, d.Diff)
	}
	if !d.Expanded {
		t.Fatal("the running turn's diff card must open itself")
	}

	// The next sent user message ends the turn: the card folds to its stats.
	snap.Messages = append(snap.Messages, session.Message{
		ID: "u2", Role: session.RoleUser, State: session.StateComplete, Text: "next",
	})
	entries, _, _ = m.Sync(entries, ids, snap)
	d = entries[1].(*block.DiffBlock)
	if d.Expanded {
		t.Fatal("a finished turn's diff card must fold")
	}
}

func TestQueuedUserMessageDoesNotFoldTheDiffCard(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := diffSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)

	snap.Messages = append(snap.Messages, session.Message{
		ID: "u2", Role: session.RoleUser, State: session.StateComplete,
		Text: "queued while running", Queued: true,
	})
	entries, _, _ = m.Sync(entries, ids, snap)
	d := entries[1].(*block.DiffBlock)
	if !d.Expanded {
		t.Fatal("a queued message waits behind the turn and must not fold its cards")
	}
}

func TestExplicitToggleOutlivesTheTurn(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := diffSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)
	d := entries[1].(*block.DiffBlock)

	// The user collapses the card mid-turn; the auto rule must not reopen it.
	d.Expanded = false
	d.OnToggle(false)
	entries, ids, _ = m.Sync(entries, ids, snap)
	d = entries[1].(*block.DiffBlock)
	if d.Expanded {
		t.Fatal("an explicit collapse must survive the sync")
	}

	// And an explicit expand survives the turn ending.
	d.Expanded = true
	d.OnToggle(true)
	snap.Messages = append(snap.Messages, session.Message{
		ID: "u2", Role: session.RoleUser, State: session.StateComplete, Text: "next",
	})
	entries, _, _ = m.Sync(entries, ids, snap)
	d = entries[1].(*block.DiffBlock)
	if !d.Expanded {
		t.Fatal("an explicit expand must survive the turn ending")
	}
}

func TestWriteRoutesToDiffCardAndGrepStaysGeneric(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := session.Snapshot{
		Messages: []session.Message{
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{
					{Type: session.BlockToolUse, ID: "w1", Name: "write", Input: "new.go"},
					{Type: session.BlockToolUse, ID: "g1", Name: "grep", Input: "pat"},
				},
			},
		},
		Tools: map[string]session.ToolRun{
			"w1": {ToolUseID: "w1", Name: "write", Status: session.ToolDone, Detail: "new.go", Output: mapperDiff},
			"g1": {ToolUseID: "g1", Name: "grep", Status: session.ToolDone, Detail: `"pat" — 3 matches in 2 files`},
		},
	}
	entries, _, _ := m.Sync(nil, nil, snap)
	if _, ok := entries[0].(*block.DiffBlock); !ok {
		t.Fatalf("write must render a diff card, got %T", entries[0])
	}
	if _, ok := entries[1].(*block.ToolBlock); !ok {
		t.Fatalf("grep stays a generic tool row, got %T", entries[1])
	}
}
