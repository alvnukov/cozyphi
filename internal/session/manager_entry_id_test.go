package session_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func newEntryIDManager(t *testing.T) *session.Manager {
	t.Helper()
	return session.NewManager(t.TempDir())
}

// The caller that drew the transcript row owns the entry id. Without this the
// row and the entry behind it drift apart the moment the session is reopened.
func TestAppendKeepsTheCallersEntryID(t *testing.T) {
	manager := newEntryIDManager(t)

	id, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "hello",
		EntryID: "0f1e2d3c4b5a6978",
	})
	require.NoError(t, err)
	assert.Equal(t, "0f1e2d3c4b5a6978", id, "the manager records the entry under the id it was given")

	entries := manager.BuildContext()
	require.Len(t, entries, 1)
	assert.Equal(t, "0f1e2d3c4b5a6978", entries[0].GetID())
}

// An empty id is the ordinary case for everything nobody draws a row for:
// tool results, autonomous turns, background deliveries.
func TestAppendMintsWhenNoEntryIDIsGiven(t *testing.T) {
	manager := newEntryIDManager(t)

	id, err := manager.Append(llm.Message{Role: llm.RoleUser, Content: "hello"})
	require.NoError(t, err)
	assert.NotEmpty(t, id, "an entry always ends up with an id")
}

// Reusing an id would put the log back where this mechanism started, with a
// row and an entry under different names and nothing saying so, so the append
// is refused instead of quietly minting a replacement.
func TestAppendRefusesATakenEntryID(t *testing.T) {
	manager := newEntryIDManager(t)

	first, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "hello",
		EntryID: "9a8b7c6d5e4f3021",
	})
	require.NoError(t, err)

	_, err = manager.Append(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "again",
		EntryID: first,
	})
	require.Error(t, err, "a taken id is refused out loud")
	assert.Contains(t, err.Error(), first)

	assert.Len(t, manager.BuildContext(), 1, "the refused message never reached the log")
}
