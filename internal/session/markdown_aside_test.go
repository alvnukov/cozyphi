package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// An exported side question is marked as one, so a reader of the file can
// tell it from the conversation it sits in.
func TestMarkdownMarksASideQuestion(t *testing.T) {
	var snap Snapshot
	snap = Apply(snap, UserAppend{Text: "hello"})
	snap = Apply(snap, AssistantMessageUpdate{Message: Message{ID: "a1", State: StateComplete, Text: "hi there"}})
	snap = Apply(snap, AsideUpdate{
		ID: "x1", Question: "what did I ask?", Answer: "you said hello", State: StateComplete,
	})

	want := "## User\n\nhello\n\n## Assistant\n\nhi there\n\n" +
		"## Side question (btw)\n\n> what did I ask?\n\nyou said hello\n"
	require.Equal(t, want, Markdown(snap.Messages))
}
