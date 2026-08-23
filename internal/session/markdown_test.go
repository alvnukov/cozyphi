package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownRendersConversation(t *testing.T) {
	var snap Snapshot
	snap = Apply(snap, UserAppend{Text: "hello"})
	snap = Apply(snap, AssistantMessageUpdate{Message: Message{
		ID:    "a1",
		State: StateComplete,
		Text:  "hi there",
	}})

	got := Markdown(snap.Messages)
	want := "## User\n\nhello\n\n## Assistant\n\nhi there\n"
	require.Equal(t, want, got)
}

func TestMarkdownSkipsEmptyAndNonConversationRows(t *testing.T) {
	var snap Snapshot
	snap = Apply(snap, UserAppend{Text: "hello"})
	snap = Apply(snap, CompactionComplete{ID: "c1"})
	snap = Apply(snap, AssistantMessageUpdate{Message: Message{
		ID:    "a1",
		State: StateError,
		Text:  "",
	}})

	got := Markdown(snap.Messages)
	require.Equal(t, "## User\n\nhello\n", got, "compaction markers and empty text produce no rows")
}

func TestMarkdownEmptySession(t *testing.T) {
	require.Empty(t, Markdown(nil))
}
