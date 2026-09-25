package session_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func rowState(t *testing.T, snap session.Snapshot, id string) session.State {
	t.Helper()
	for _, msg := range snap.Messages {
		if msg.ID == id {
			return msg.State
		}
	}
	t.Fatalf("no row %s", id)
	return 0
}

// Esc cancels the turn that streams, and a side question is no turn: its
// row is ended by the update that carries its id, whether or not it is the
// last row of the feed.
func TestOnlyItsOwnUpdateEndsAnAsideRow(t *testing.T) {
	snap := session.Apply(session.Snapshot{}, session.AsideUpdate{
		ID: "a1", Question: "side?", Answer: "part", State: session.StateStreaming,
	})
	snap = session.Apply(snap, session.CancelStreaming{})
	assert.Equal(t, session.StateStreaming, rowState(t, snap, "a1"), "the cancel of a turn is not aimed at the aside")

	snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
		ID: "turn", State: session.StateStreaming, Text: "hello",
	}})
	snap = session.Apply(snap, session.CancelStreaming{})
	assert.Equal(t, session.StateCancelled, rowState(t, snap, "turn"))
	assert.Equal(t, session.StateStreaming, rowState(t, snap, "a1"))

	snap = session.Apply(snap, session.AsideUpdate{
		ID: "a1", Question: "side?", Answer: "part", State: session.StateCancelled,
	})
	require.Len(t, snap.Messages, 2)
	assert.Equal(t, session.StateCancelled, rowState(t, snap, "a1"))
	assert.Equal(t, "a1", snap.Messages[0].ID, "the row stays where it was drawn")
}
