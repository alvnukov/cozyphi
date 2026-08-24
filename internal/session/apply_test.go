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
			{
				ID: "a", Role: RoleAssistant, State: StateStreaming,
				Content: []ContentBlock{{Type: BlockText, Text: "partial"}},
			},
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
			{
				ID: "a1", Role: RoleAssistant, State: StateComplete,
				Content: []ContentBlock{
					{Type: BlockThinking, Text: "plan"},
					{Type: BlockText, Text: "hello"},
					{Type: BlockToolUse, ID: "t1", Name: "Bash", Input: "ls"},
				},
			},
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
	s = Apply(s, CompactionComplete{
		ID: "c1",
		Compaction: Compaction{
			TokensBefore:       56000,
			TokensAfter:        8000,
			MessagesSummarized: 12,
			MessagesKept:       4,
		},
	})
	if s.Compacting {
		t.Fatal("should clear compacting")
	}
	if len(s.Messages) != 2 || s.Messages[1].Role != RoleCompaction {
		t.Fatalf("marker: %+v", s.Messages)
	}
	if s.Messages[1].Text != "Compacted 12 messages · 56k → ~8k context · 4 kept" {
		t.Fatalf("marker text = %q", s.Messages[1].Text)
	}
	items := Project(s)
	if len(items) < 2 || items[len(items)-1].Kind != ItemCompaction {
		t.Fatalf("project: %+v", items)
	}
	if items[len(items)-1].Text != s.Messages[1].Text {
		t.Fatalf("projected report = %q", items[len(items)-1].Text)
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

func TestLocalBash(t *testing.T) {
	var s Snapshot
	s = Apply(s, LocalBashStart{ID: "b1", Command: "echo hi"})
	if len(s.Messages) != 1 || s.Messages[0].Role != RoleLocalBash {
		t.Fatalf("msg: %+v", s.Messages)
	}
	if !s.Tools["b1"].Local || s.Tools["b1"].Status != ToolInProgress {
		t.Fatalf("tool: %+v", s.Tools["b1"])
	}
	if IsStreaming(s) || HasRunningTools(s) {
		t.Fatal("local bash must not count as agent streaming")
	}

	items := Project(s)
	if len(items) != 1 || items[0].Kind != ItemTool || items[0].ToolName != "bash" {
		t.Fatalf("project: %+v", items)
	}

	s = Apply(s, ToolData{Run: ToolRun{
		ToolUseID: "b1",
		Status:    ToolDone,
		Output:    "hi\n",
		ExitCode:  0,
		Local:     true,
	}})
	if s.Tools["b1"].Status != ToolDone || !s.Tools["b1"].Local {
		t.Fatalf("done: %+v", s.Tools["b1"])
	}

	// CancelStreaming must leave local bash alone.
	s = Apply(s, LocalBashStart{ID: "b2", Command: "sleep 9"})
	s = Apply(s, CancelStreaming{})
	if s.Tools["b2"].Status != ToolInProgress {
		t.Fatalf("cancel must skip local: %+v", s.Tools["b2"])
	}
}

func TestReducerOwnsInputSnapshot(t *testing.T) {
	source := Snapshot{
		Messages: []Message{{ID: "a1", Role: RoleAssistant, State: StateStreaming}},
		Tools: map[string]ToolRun{
			"t1": {ToolUseID: "t1", Status: ToolInProgress},
		},
	}
	reducer := NewReducer(source)
	reducer.Apply(AssistantMessageUpdate{Message: Message{ID: "a1", State: StateComplete}})
	reducer.Apply(ToolData{Run: ToolRun{ToolUseID: "t1", Status: ToolDone}})

	if source.Messages[0].State != StateStreaming {
		t.Fatalf("source message state = %s, want streaming", source.Messages[0].State)
	}
	if source.Tools["t1"].Status != ToolInProgress {
		t.Fatalf("source tool status = %s, want in-progress", source.Tools["t1"].Status)
	}
}
