package session

import "testing"

// TestProjectCoalescesThinkingBlocks: several consecutive reasoning blocks in
// one message project to a single thinking row (one "Thinking" header, not N).
func TestProjectCoalescesThinkingBlocks(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete,
		Content: []ContentBlock{
			{Type: BlockThinking, Text: "first"},
			{Type: BlockThinking, Text: "second"},
			{Type: BlockText, Text: "answer"},
		},
	}}}
	items := Project(s)
	if len(items) != 2 {
		t.Fatalf("len=%d items=%+v", len(items), items)
	}
	if items[0].Kind != ItemThinking || items[0].Thinking != "first\n\nsecond" {
		t.Fatalf("thinking item: %+v", items[0])
	}
	if items[0].ID != "a1-thinking-0" {
		t.Fatalf("merged id = %q, want first block id", items[0].ID)
	}
	if items[1].Kind != ItemAssistant || items[1].Text != "answer" {
		t.Fatalf("text item: %+v", items[1])
	}
}

// TestProjectCoalescesThinkingAcrossMessages: thinking-only assistant
// messages in a row (the screenshot's stack of "• Thinking •") collapse into
// one row; the trailing block keeps its streaming flag.
func TestProjectCoalescesThinkingAcrossMessages(t *testing.T) {
	s := Snapshot{Messages: []Message{
		{ID: "u1", Role: RoleUser, Text: "тест"},
		{
			ID: "a1", Role: RoleAssistant, State: StateComplete,
			Content: []ContentBlock{{Type: BlockThinking, Text: "one"}},
		},
		{
			ID: "a2", Role: RoleAssistant, State: StateStreaming,
			Content: []ContentBlock{{Type: BlockThinking, Text: "two"}},
		},
	}}
	items := Project(s)
	thinking := 0
	for _, it := range items {
		if it.Kind == ItemThinking {
			thinking++
			if it.Thinking != "one\n\ntwo" {
				t.Fatalf("merged text = %q", it.Thinking)
			}
			if !it.Streaming {
				t.Fatal("merged row must keep trailing streaming flag")
			}
			if it.ID != "a1-thinking-0" {
				t.Fatalf("merged id = %q, want first row id", it.ID)
			}
		}
	}
	if thinking != 1 {
		t.Fatalf("thinking rows = %d, want 1 (items=%+v)", thinking, items)
	}
}

// TestProjectKeepsSeparatedThinking: thinking split by a tool call stays in
// separate rows — only uninterrupted runs collapse.
func TestProjectKeepsSeparatedThinking(t *testing.T) {
	s := Snapshot{Messages: []Message{{
		ID: "a1", Role: RoleAssistant, State: StateComplete,
		Content: []ContentBlock{
			{Type: BlockThinking, Text: "before"},
			{Type: BlockToolUse, ID: "t1", Name: "Read", Input: "x.go"},
			{Type: BlockThinking, Text: "after"},
		},
	}}}
	items := Project(s)
	if len(items) != 3 {
		t.Fatalf("len=%d items=%+v", len(items), items)
	}
	if items[0].Kind != ItemThinking || items[0].Thinking != "before" {
		t.Fatalf("first: %+v", items[0])
	}
	if items[1].Kind != ItemTool {
		t.Fatalf("middle must be tool: %+v", items[1])
	}
	if items[2].Kind != ItemThinking || items[2].Thinking != "after" {
		t.Fatalf("last: %+v", items[2])
	}
}
