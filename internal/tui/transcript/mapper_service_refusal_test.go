package transcript_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// refusalSnap builds one turn whose read call ended in the given tool run.
func refusalSnap(run session.ToolRun) session.Snapshot {
	run.ToolUseID = "t1"
	run.Name = "read"
	return session.Snapshot{
		Messages: []session.Message{
			{ID: "u1", Role: session.RoleUser, State: session.StateComplete, Text: "go"},
			{
				ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
				Content: []session.ContentBlock{
					{Type: session.BlockToolUse, ID: "t1", Name: "read", Input: "CHANGELOG.md"},
				},
			},
		},
		Tools: map[string]session.ToolRun{"t1": run},
	}
}

func TestSkillPreloadRefusalStaysOutOfTheFeed(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	reason := plangate.ReasonSkillPreload + "\n\n## Skill: tests\nrun the suite"
	entries, _, _ := m.Sync(nil, nil, refusalSnap(session.ToolRun{
		Status: session.ToolRejected, Detail: "CHANGELOG.md", Error: reason, Output: reason,
	}))
	for _, w := range entries {
		if tb, ok := w.(*block.ToolBlock); ok && tb.Name == "read" {
			t.Fatal("the skill-preload refusal must not render a tool row")
		}
	}
}

func TestBatchSkillPreloadRefusalStaysOutOfTheFeed(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, refusalSnap(session.ToolRun{
		Status: session.ToolRejected, Detail: "CHANGELOG.md",
		Error: plangate.ReasonBatchSkillPreload, Output: plangate.ReasonBatchSkillPreload,
	}))
	for _, w := range entries {
		if tb, ok := w.(*block.ToolBlock); ok && tb.Name == "read" {
			t.Fatal("the batch-tail refusal must not render a tool row")
		}
	}
}

func TestGenuineRejectionStillRenders(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, _, _ := m.Sync(nil, nil, refusalSnap(session.ToolRun{
		Status: session.ToolRejected, Detail: "CHANGELOG.md",
		Error: "denied by the user", Output: "denied by the user",
	}))
	var found *block.ToolBlock
	for _, w := range entries {
		if tb, ok := w.(*block.ToolBlock); ok && tb.Name == "read" {
			found = tb
		}
	}
	if found == nil {
		t.Fatal("a genuine rejection must keep its tool row")
	}
	if found.Error != "denied by the user" {
		t.Fatalf("row error = %q", found.Error)
	}
}

// The refusal lands on a row the feed already showed as running: the row must
// leave the feed on the next sync instead of turning into a scary rejection.
func TestRefusalRemovesTheRowItWasRunningAs(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, refusalSnap(session.ToolRun{
		Status: session.ToolInProgress, Detail: "CHANGELOG.md",
	}))
	seen := false
	for _, w := range entries {
		if tb, ok := w.(*block.ToolBlock); ok && tb.Name == "read" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("precondition: the running call renders a row")
	}

	reason := plangate.ReasonSkillPreload + "\n\nbody"
	entries, _, _ = m.Sync(entries, ids, refusalSnap(session.ToolRun{
		Status: session.ToolRejected, Detail: "CHANGELOG.md", Error: reason, Output: reason,
	}))
	for _, w := range entries {
		if tb, ok := w.(*block.ToolBlock); ok && tb.Name == "read" {
			t.Fatal("the refusal must remove the row, not restyle it")
		}
	}
}
