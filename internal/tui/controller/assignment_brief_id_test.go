package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// userRowIDs drains a bus and returns the id of every user transcript row it
// carried, in order.
func userRowIDs(bus *Bus) []string {
	var out []string
	for _, m := range bus.Drain() {
		ev, ok := m.(SessionEventMsg)
		if !ok {
			continue
		}
		if appended, ok := ev.Event.(session.UserAppend); ok {
			out = append(out, appended.ID)
		}
	}
	return out
}

// The brief is the third place a row id is minted, next to the submitter and
// the streaming round, and it is the one nobody types. Its row has to name the
// session entry the same turn wrote, or a reopened sub-agent would anchor on a
// different message than the live screen does.
func TestAssignmentBriefRowNamesItsEntry(t *testing.T) {
	server, _ := textSSEServer(t)
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.runtime.EnableInteractiveChildren()
	parent.Cancel()
	child, _ := spawnRunningChild(t, parent, "queue cleanup", "refactor the queue")

	ids := userRowIDs(child.Bus)
	require.Len(t, ids, 1, "the brief draws exactly one row")
	require.NotEmpty(t, ids[0], "a row nobody can address is a row no rewind reaches")

	entries := child.Controller.engine.Session().PathEntries()
	var firstUser string
	for _, entry := range entries {
		message, ok := entry.(session.SessionMessageEntry)
		if !ok || message.Message.Role != llm.RoleUser {
			continue
		}
		firstUser = entry.GetID()
		break
	}
	assert.Equal(t, firstUser, ids[0], "the brief row and its session entry are one message")

	replayed := transcript.ReplaySnapshot(entries)
	require.NotEmpty(t, replayed.Messages)
	assert.Equal(t, ids[0], replayed.Messages[0].ID,
		"reopening the child must land on the same brief the live screen opened on")
}
