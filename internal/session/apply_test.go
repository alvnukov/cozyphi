package session

import "testing"

func TestApplyStreamingUpdates(t *testing.T) {
	var s Snapshot
	s = Apply(s, UserAppend{ID: "u1", Text: "hello"})
	if len(s.Messages) != 1 || s.Messages[0].Role != RoleUser {
		t.Fatalf("user: %+v", s.Messages)
	}

	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "Hi"}},
	}})
	if len(s.Messages) != 2 || !IsStreaming(s) {
		t.Fatalf("start: %+v", s.Messages)
	}

	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "Hi there"}},
	}})
	if len(s.Messages) != 2 || s.Messages[1].FlatText() != "Hi there" {
		t.Fatalf("delta replace: %+v", s.Messages)
	}

	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete,
		Content: []ContentBlock{{Type: BlockText, Text: "Hi there!"}},
	}})
	if IsStreaming(s) || s.Messages[1].State != StateComplete {
		t.Fatalf("complete: %+v", s.Messages)
	}
}

func TestCancelStreaming(t *testing.T) {
	s := Snapshot{
		Messages: []Message{
			{ID: "u", Role: RoleUser, Text: "x"},
			{ID: "a", Role: RoleAssistant, State: StateStreaming,
				Content: []ContentBlock{{Type: BlockText, Text: "partial"}}},
		},
		Tools: map[string]ToolRun{
			"t1": {ToolUseID: "t1", Status: ToolInProgress},
		},
	}
	s = Apply(s, CancelStreaming{})
	if s.Messages[1].State != StateCancelled {
		t.Fatalf("assistant: %+v", s.Messages[1])
	}
	if s.Tools["t1"].Status != ToolCancelled {
		t.Fatalf("tool: %+v", s.Tools["t1"])
	}
}

func TestToolDataAndSecondTurn(t *testing.T) {
	var s Snapshot
	s = Apply(s, UserAppend{Text: "go"})
	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete, StopReason: StopToolUse,
		Content: []ContentBlock{
			{Type: BlockText, Text: "calling"},
			{Type: BlockToolUse, ID: "t1", Name: "Read", Input: "a.go", Complete: true},
		},
	}})
	if s.Tools["t1"].Status != ToolInProgress {
		t.Fatalf("synthetic tool: %+v", s.Tools)
	}
	if s.Tools["t1"].Name != "Read" {
		t.Fatalf("expected Name=Read from tool_use block, got %q", s.Tools["t1"].Name)
	}
	s = Apply(s, ToolData{Run: ToolRun{ToolUseID: "t1", Status: ToolDone, Output: "ok"}})
	if s.Tools["t1"].Status != ToolDone || s.Tools["t1"].Output != "ok" {
		t.Fatalf("tool done: %+v", s.Tools["t1"])
	}
	if s.Tools["t1"].Name != "Read" {
		t.Fatalf("expected Name preserved across ToolData, got %q", s.Tools["t1"].Name)
	}
	s = Apply(s, AssistantMessageUpdate{Message: Message{
		ID: "a2", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "done"}},
	}})
	if len(s.Messages) != 3 || s.Messages[2].ID != "a2" {
		t.Fatalf("second turn: %+v", s.Messages)
	}
}

func TestProjectOrder(t *testing.T) {
	s := Snapshot{
		Messages: []Message{
			{ID: "u1", Role: RoleUser, Text: "hi"},
			{ID: "a1", Role: RoleAssistant, State: StateComplete,
				Content: []ContentBlock{
					{Type: BlockThinking, Text: "plan"},
					{Type: BlockText, Text: "hello"},
					{Type: BlockToolUse, ID: "t1", Name: "Bash", Input: "ls"},
				}},
		},
		Tools: map[string]ToolRun{
			"t1": {ToolUseID: "t1", Status: ToolDone, Output: "a\n", Detail: "ls"},
		},
	}
	items := Project(s)
	if len(items) != 4 {
		t.Fatalf("len=%d %+v", len(items), items)
	}
	if items[0].Kind != ItemUser || items[1].Kind != ItemThinking ||
		items[2].Kind != ItemAssistant || items[3].Kind != ItemTool {
		t.Fatalf("order: %+v", items)
	}
	if items[3].ToolRun.Status != ToolDone || items[3].ToolRun.Output != "a\n" {
		t.Fatalf("tool item: %+v", items[3])
	}
}

func TestCompactionEvents(t *testing.T) {
	var s Snapshot
	s = Apply(s, UserAppend{Text: "hi"})
	s = Apply(s, CompactionStarted{})
	if !s.Compacting || !IsStreaming(s) {
		t.Fatalf("compacting: %+v", s)
	}
	s = Apply(s, CompactionComplete{ID: "c1"})
	if s.Compacting {
		t.Fatal("should clear compacting")
	}
	if len(s.Messages) != 2 || s.Messages[1].Role != RoleCompaction {
		t.Fatalf("marker: %+v", s.Messages)
	}
	items := Project(s)
	if len(items) < 2 || items[len(items)-1].Kind != ItemCompaction {
		t.Fatalf("project: %+v", items)
	}

	s = Apply(s, CompactionStarted{})
	s = Apply(s, CompactionComplete{ID: "c2", Failed: true})
	if s.Compacting {
		t.Fatal("failed should clear")
	}
	nMarkers := 0
	for _, m := range s.Messages {
		if m.Role == RoleCompaction {
			nMarkers++
		}
	}
	if nMarkers != 1 {
		t.Fatalf("markers=%d", nMarkers)
	}
}
