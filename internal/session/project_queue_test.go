package session

import "testing"

// TestProjectMarksQueuedUser: a submit accepted behind a running turn must
// surface in the transcript as a queued row, not a plain user row, so the UI
// can show that the message is waiting for the model to stop.
func TestProjectMarksQueuedUser(t *testing.T) {
	snap := Apply(Snapshot{}, UserAppend{ID: "u1", Text: "hello", Queued: true})
	items := Project(snap)
	if len(items) != 1 || items[0].Kind != ItemUser {
		t.Fatalf("user item missing: %+v", items)
	}
	if !items[0].Queued {
		t.Fatalf("queued user must project as queued: %+v", items[0])
	}
	if items[0].Text != "hello" {
		t.Fatalf("text = %q, want hello", items[0].Text)
	}
}

// TestProjectQueuedFlagDefaultsFalse: an ordinary submit during idle must stay
// a plain user row.
func TestProjectQueuedFlagDefaultsFalse(t *testing.T) {
	snap := Apply(Snapshot{}, UserAppend{ID: "u1", Text: "hello"})
	items := Project(snap)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(items), items)
	}
	if items[0].Queued {
		t.Fatalf("non-queued user must not project as queued: %+v", items[0])
	}
}
