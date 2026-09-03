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

func TestEditRoutesToDiffCardOpenUnderTheDefaultSwitch(t *testing.T) {
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
		t.Fatal("with expand-edits on (the default) a finished diff card renders open")
	}

	// The turn ending changes nothing: the switch, not the turn, owns the
	// default now.
	snap.Messages = append(snap.Messages, session.Message{
		ID: "u2", Role: session.RoleUser, State: session.StateComplete, Text: "next",
	})
	entries, _, _ = m.Sync(entries, ids, snap)
	d = entries[1].(*block.DiffBlock)
	if !d.Expanded {
		t.Fatal("the card must stay open when the turn ends")
	}
}

func TestExpandEditsOffRendersDiffCardsFolded(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	m.SetExpandEdits(false, nil, nil)
	entries, _, _ := m.Sync(nil, nil, diffSnap())
	d := entries[1].(*block.DiffBlock)
	if d.Expanded {
		t.Fatal("with expand-edits off a new diff card must render folded")
	}
}

func TestExpandEditsOffFoldsTheFeedAtOnce(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	snap := diffSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)
	d := entries[1].(*block.DiffBlock)
	if !d.Expanded {
		t.Fatal("precondition: the card starts open")
	}

	changed := m.SetExpandEdits(false, entries, ids)
	if len(changed) != 1 || changed[0] != 1 {
		t.Fatalf("changed rows = %v, want [1]", changed)
	}
	if d.Expanded {
		t.Fatal("switching expand-edits off must fold the card at once")
	}
	// The fold is pinned: the next sync must not reopen it.
	entries, _, _ = m.Sync(entries, ids, snap)
	if entries[1].(*block.DiffBlock).Expanded {
		t.Fatal("the fold must survive the next sync")
	}
}

func TestExpandEditsOnTouchesOnlyFutureCards(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	m.SetExpandEdits(false, nil, nil)
	snap := diffSnap()
	entries, ids, _ := m.Sync(nil, nil, snap)

	if changed := m.SetExpandEdits(true, entries, ids); len(changed) != 0 {
		t.Fatalf("switching on must not touch the feed, changed %v", changed)
	}
	d := entries[1].(*block.DiffBlock)
	if d.Expanded {
		t.Fatal("an existing folded card must stay folded when the switch turns on")
	}
	entries, ids, _ = m.Sync(entries, ids, snap)
	if entries[1].(*block.DiffBlock).Expanded {
		t.Fatal("the pinned fold must survive the next sync")
	}

	// A card the feed has not seen yet is born under the new default.
	snap.Messages = append(snap.Messages, session.Message{
		ID: "a2", Role: session.RoleAssistant, State: session.StateComplete,
		Content: []session.ContentBlock{
			{Type: session.BlockToolUse, ID: "t2", Name: "edit", Input: "other.go"},
		},
	})
	snap.Tools["t2"] = session.ToolRun{
		ToolUseID: "t2", Name: "edit", Status: session.ToolDone,
		Detail: "other.go", Output: mapperDiff,
	}
	entries, _, _ = m.Sync(entries, ids, snap)
	last := entries[len(entries)-1].(*block.DiffBlock)
	if !last.Expanded {
		t.Fatal("a future card must be born open under the switched-on default")
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
