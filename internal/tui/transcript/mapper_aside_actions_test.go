package transcript_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// Rewind, fork and btw name entries of the conversation, and a side question
// is none: its row offers none of them, finished or not.
func TestAnAsideRowOffersNoMessageActions(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	var got anchors
	got.wireOffering(m, "u1", "a1", "x1")

	snap := session.Apply(session.Snapshot{}, session.UserAppend{ID: "u1", Text: "hello"})
	snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
		ID: "a1", State: session.StateComplete, Text: "hi", Model: "m",
		Content: []session.ContentBlock{{Type: session.BlockText, Text: "hi"}},
	}})
	snap = session.Apply(snap, session.AsideUpdate{
		ID: "x1", Question: "side?", Answer: "an answer", State: session.StateComplete, Model: "m",
	})
	entries, ids, _ := m.Sync(nil, nil, snap)
	// Matched by prefix: whatever widget draws the aside, its row id starts
	// with the aside's own.
	at := slices.IndexFunc(ids, func(id string) bool { return strings.HasPrefix(id, "x1") })
	if at < 0 {
		t.Fatalf("no row for the aside in %v", ids)
	}
	for _, label := range []string{"rewind", "fork"} {
		if clickStrip(t, entries[at], label) {
			t.Fatalf("the aside row offers %q", label)
		}
	}
	// The title says btw too; a click there folds the answer and asks nothing.
	clickStrip(t, entries[at], "btw")
	if len(got.rewind)+len(got.fork)+len(got.aside) != 0 {
		t.Fatalf("an action fired from the aside row: %+v", got)
	}
}
