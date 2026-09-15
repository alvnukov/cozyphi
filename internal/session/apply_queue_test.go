package session

import "testing"

// TestApplyUserPromotedAppendsAtEnd covers submit-while-streaming under the
// delivery-order transcript: the prompt queued behind the running turn has
// no row while it waits; when the engine delivers it, UserPromoted appends
// the user row at the END — after every row the in-flight turn produced in
// the meantime — and the streaming turn still completes in place, with no
// duplicated assistant row and no stuck "streaming" ghost.
func TestApplyUserPromotedAppendsAtEnd(t *testing.T) {
	r := NewReducer(Snapshot{})
	r.Apply(UserAppend{ID: "u1", Text: "first"})
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "par"}},
	}})

	// The turn keeps producing rows while the second prompt waits in the
	// controller's queue: nothing about it appears in the transcript yet.
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateStreaming,
		Content: []ContentBlock{{Type: BlockText, Text: "partial"}},
	}})

	// Delivery: the queued prompt's row lands at the end of the feed.
	r.Apply(UserPromoted{ID: "u2", Text: "second"})

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
	if snap.Messages[2].Role != RoleUser || snap.Messages[2].Text != "second" || snap.Messages[2].ID != "u2" {
		t.Fatalf("promoted user row wrong: %+v", snap.Messages[2])
	}
	if IsStreaming(snap) {
		t.Fatalf("pipeline must be idle after the turn completes: %+v", snap.Messages)
	}
}

// TestApplyUserPromotedUnknownIDStillAppends: delivery is the fact, the id
// is bookkeeping — a promote for an id no row carries still appends the
// user row at the end.
func TestApplyUserPromotedUnknownIDStillAppends(t *testing.T) {
	r := NewReducer(Snapshot{})
	r.Apply(UserAppend{ID: "u1", Text: "first"})
	r.Apply(UserPromoted{ID: "missing", Text: "second"})

	snap := r.Snapshot()
	if len(snap.Messages) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(snap.Messages), snap.Messages)
	}
	if snap.Messages[1].Role != RoleUser || snap.Messages[1].Text != "second" || snap.Messages[1].ID != "missing" {
		t.Fatalf("promoted row wrong: %+v", snap.Messages[1])
	}
}

// TestApplySameIDUpdateAfterPromotedUserAppend: a late same-ID update for a
// completed turn still replaces that turn, not the promoted user row below
// it and not a fresh duplicate.
func TestApplySameIDUpdateAfterPromotedUserAppend(t *testing.T) {
	r := NewReducer(Snapshot{})
	r.Apply(UserAppend{ID: "u1", Text: "first"})
	r.Apply(AssistantMessageUpdate{Message: Message{
		ID: "a1", State: StateComplete,
		Content: []ContentBlock{{Type: BlockText, Text: "done"}},
	}})
	r.Apply(UserPromoted{ID: "u2", Text: "second"})

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
		t.Fatalf("promoted user row wrong: %+v", snap.Messages[2])
	}
}
