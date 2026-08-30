package session

import "testing"

// TestCompactNotice pins the row a delivered compaction reminder leaves
// behind: visible in the transcript, local like a watch row, and red once
// the ladder reaches the hard strike.
func TestCompactNotice(t *testing.T) {
	var s Snapshot
	s = Apply(s, CompactNotice{Label: "context pressure ~151k of 1M (15%) · reminder 1 of 4"})

	if len(s.Messages) != 1 || s.Messages[0].Role != RoleNotice {
		t.Fatalf("msg: %+v", s.Messages)
	}
	if s.Messages[0].Text != "context pressure ~151k of 1M (15%) · reminder 1 of 4" {
		t.Fatalf("text: %q", s.Messages[0].Text)
	}
	run := s.Tools[s.Messages[0].ID]
	if run.Name != "context" || !run.Local || run.Status != ToolDone {
		t.Fatalf("tool: %+v", run)
	}

	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemTool || items[0].ToolName != "context" {
		t.Fatalf("items: %+v", items)
	}
	if !items[0].ToolRun.Local {
		t.Fatalf("not local: %+v", items[0].ToolRun)
	}
}

// TestCompactNoticeHardMarksTheRow checks the hard strike renders as an
// error-tinted row, so the blocking ladder is legible without colors.
func TestCompactNoticeHardMarksTheRow(t *testing.T) {
	s := Apply(Snapshot{}, CompactNotice{Label: "context limit · reminder 3 — tools blocked", Hard: true})

	if len(s.Messages) != 1 {
		t.Fatalf("msg: %+v", s.Messages)
	}
	if run := s.Tools[s.Messages[0].ID]; run.Status != ToolError {
		t.Fatalf("hard: %+v", run)
	}
}

// TestCompactNoticeGetsAnIDWhenTheCallerHasNone mirrors the watch fallback:
// engine emitters may not bother minting IDs.
func TestCompactNoticeGetsAnIDWhenTheCallerHasNone(t *testing.T) {
	s := Apply(Snapshot{}, CompactNotice{Label: "nudge"})

	if s.Messages[0].ID == "" {
		t.Fatalf("id: %+v", s.Messages[0])
	}
}
