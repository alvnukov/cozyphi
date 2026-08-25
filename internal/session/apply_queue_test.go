package session

import "testing"

// TestApplyQueuedUserAppendDuringStreaming covers submit-while-streaming: the
// user prompt is accepted (queued behind the running turn) and the transcript
// shows it immediately, but the in-flight assistant turn must still complete
// in place — no duplicated assistant row and no stuck "streaming" ghost.
func TestApplyQueuedUserAppendDuringStreaming(t *testing.T) {
	r := NewReducer(Snapshot{})
	r.Apply(UserAppend{ID: "u1", Text: "first"})
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "par"}},
	}})

	// The user submits while the assistant is streaming.
	r.Apply(UserAppend{ID: "u2", Text: "second", Queued: true})

	// The streaming assistant keeps streaming, then completes.
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "partial"}},
	}})
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete,
		Content: []ContentBlock{{Type: BlockText, Text: "done"}},
	}})

	snap := r.Snapshot()
	if len(snap.Messages) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(snap.Messages), snap.Messages)
	}
	if snap.Messages[1].Role != RoleAssistant || snap.Messages[1].State != StateComplete {
		t.Fatalf("assistant row wrong: %+v", snap.Messages[1])
	}
	if snap.Messages[1].FlatText() != "done" {
		t.Fatalf("assistant text = %q, want done", snap.Messages[1].FlatText())
	}
	if snap.Messages[2].Role != RoleUser || snap.Messages[2].Text != "second" {
		t.Fatalf("queued user row wrong: %+v", snap.Messages[2])
	}
	if IsStreaming(snap) {
		t.Fatalf("pipeline must be idle after the turn completes: %+v", snap.Messages)
	}
}

// TestApplySameIDUpdateAfterQueuedUserAppend: a late same-ID update for a
// completed turn still replaces that turn, not the queued user row below it
// and not a fresh duplicate. This is the idle-state sibling of the streaming
// scan above.
func TestApplySameIDUpdateAfterQueuedUserAppend(t *testing.T) {
	r := NewReducer(Snapshot{})
	r.Apply(UserAppend{ID: "u1", Text: "first"})
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete,
		Content: []ContentBlock{{Type: BlockText, Text: "done"}},
	}})
	r.Apply(UserAppend{ID: "u2", Text: "second"})

	// A late update for the completed turn (same ID) must replace a1 in place.
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete,
		Content: []ContentBlock{{Type: BlockText, Text: "done, amended"}},
	}})

	snap := r.Snapshot()
	if len(snap.Messages) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(snap.Messages), snap.Messages)
	}
	if snap.Messages[1].FlatText() != "done, amended" {
		t.Fatalf("assistant text = %q, want done, amended", snap.Messages[1].FlatText())
	}
	if snap.Messages[2].Role != RoleUser || snap.Messages[2].Text != "second" {
		t.Fatalf("queued user row wrong: %+v", snap.Messages[2])
	}
}
