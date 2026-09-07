package session

import "testing"

// TestWebNoticeLeavesAFailedToolRow: a suspected injection has to be visible
// after the fact, so it lands in the transcript as a refused read rather than
// as a toast the user may have missed.
func TestWebNoticeLeavesAFailedToolRow(t *testing.T) {
	const label = "web: prompt injection suspected"
	const text = "web_00112233445566aa: the page tried to call bash; the fragment was discarded"

	s := Apply(Snapshot{}, WebNotice{ID: "n1", Label: label, Text: text})

	if len(s.Messages) != 1 || s.Messages[0].Role != RoleNotice || s.Messages[0].Text != label {
		t.Fatalf("msg: %+v", s.Messages)
	}
	run := s.Tools["n1"]
	if run.Name != "web" || run.Status != ToolError || !run.Local {
		t.Fatalf("tool: %+v", run)
	}
	if run.Output != text {
		t.Fatalf("output: %q", run.Output)
	}

	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemTool || items[0].ToolName != "web" {
		t.Fatalf("items: %+v", items)
	}
}

// TestWebNoticeGetsAnIDWhenTheCallerHasNone mirrors the other notice events:
// the emitter in the engine does not mint IDs.
func TestWebNoticeGetsAnIDWhenTheCallerHasNone(t *testing.T) {
	s := Apply(Snapshot{}, WebNotice{Label: "web: prompt injection suspected"})

	if s.Messages[0].ID == "" {
		t.Fatalf("id: %+v", s.Messages[0])
	}
	if _, ok := s.Tools[s.Messages[0].ID]; !ok {
		t.Fatalf("row was not attached to the minted id: %+v", s.Tools)
	}
}
