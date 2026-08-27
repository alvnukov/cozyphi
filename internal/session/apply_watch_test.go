package session

import "testing"

// TestWatchFired pins the row a background watch leaves behind: visible in the
// transcript, and invisible to every check that asks whether the agent is busy.
// A watch firing while the user types must not look like a running turn.
func TestWatchFired(t *testing.T) {
	var s Snapshot
	s = Apply(s, WatchFired{ID: "w1-3", Label: "errors in deploy.log", Text: "ERROR connection refused"})

	if len(s.Messages) != 1 || s.Messages[0].Role != RoleWatch {
		t.Fatalf("msg: %+v", s.Messages)
	}
	run := s.Tools["w1-3"]
	if run.Name != "watch" || !run.Local || run.Status != ToolDone {
		t.Fatalf("tool: %+v", run)
	}
	if run.Output != "ERROR connection refused" {
		t.Fatalf("output: %q", run.Output)
	}
	if IsStreaming(s) || HasRunningTools(s) {
		t.Fatal("a watch event must not count as agent activity")
	}

	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemTool || items[0].ToolName != "watch" {
		t.Fatalf("project: %+v", items)
	}
	if items[0].ToolRun.Output != "ERROR connection refused" {
		t.Fatalf("projected output: %+v", items[0].ToolRun)
	}

	// Canceling the turn the event landed in must leave the event alone.
	s = Apply(s, CancelStreaming{})
	if s.Tools["w1-3"].Status != ToolDone {
		t.Fatalf("cancel must skip watch rows: %+v", s.Tools["w1-3"])
	}
}

func TestWatchFiredGetsAnIDWhenTheCallerHasNone(t *testing.T) {
	var s Snapshot
	s = Apply(s, WatchFired{Label: "timer", Text: "check the deploy"})
	if len(s.Messages) != 1 || s.Messages[0].ID == "" {
		t.Fatalf("msg: %+v", s.Messages)
	}
	if _, ok := s.Tools[s.Messages[0].ID]; !ok {
		t.Fatalf("tool row missing for %q: %+v", s.Messages[0].ID, s.Tools)
	}
}
