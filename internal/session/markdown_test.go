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

// An aside about an earlier message names it, and an answer the model cut
// short at a tool call says so, as the row in the feed does.
func TestMarkdownNamesTheAnchorOfASideQuestion(t *testing.T) {
	snap := Apply(Snapshot{}, AsideUpdate{
		ID: "x1", Question: "why?\nreally", AnchorPreview: "answer hi there", Answer: "because",
		SkippedTool: "bash", State: StateComplete,
	})
	want := "## Side question (btw)\n\n> why?\n> really\n\nre: answer hi there\n\nbecause\n\n" +
		AsideSkippedToolNote("bash") + "\n"
	require.Equal(t, want, Markdown(snap.Messages))
}

// An answer cut short by Esc or by an error is exported as such, so the part
// that arrived is not read as the whole answer.
func TestMarkdownSaysASideAnswerDidNotFinish(t *testing.T) {
	cancelled := Apply(Snapshot{}, AsideUpdate{ID: "x1", Question: "why?", Answer: "becau", State: StateCancelled})
	require.Equal(t, "## Side question (btw)\n\n> why?\n\n(cancelled)\n\nbecau\n", Markdown(cancelled.Messages))

	failed := Apply(Snapshot{}, AsideUpdate{
		ID: "x1", Question: "why?", Answer: "becau", Error: "provider said no", State: StateError,
	})
	require.Equal(t, "## Side question (btw)\n\n> why?\n\n(failed)\n\nbecau\n\nprovider said no\n",
		Markdown(failed.Messages))
}
