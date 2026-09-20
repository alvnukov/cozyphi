package session_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The boundaries of a session are its prompts and the answers that finished
// a turn, oldest first; everything a turn did in between is not one.
func TestTurnBoundariesListPromptsAndFinishedAnswers(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	// The plain function reports the shape of the path: all four.
	all := session.TurnBoundaries(manager.BuildContext())
	require.Len(t, all, 4)

	assert.Equal(t, first.Prompt, all[0].EntryID)
	assert.Equal(t, session.BoundaryPrompt, all[0].Kind)
	assert.Empty(t, all[0].Target, "nothing stands before the first prompt")
	assert.Equal(t, "before one", all[0].Preview)

	assert.Equal(t, first.Answer, all[1].EntryID)
	assert.Equal(t, session.BoundaryAnswer, all[1].Kind)
	assert.Equal(t, first.Answer, all[1].Target)
	assert.Equal(t, "after answer one", all[1].Preview)

	assert.Equal(t, second.Prompt, all[2].EntryID)
	assert.Equal(t, first.Answer, all[2].Target)
	assert.Equal(t, second.Answer, all[3].EntryID)

	// What the manager offers leaves out the last answer, because the cursor
	// already stands there and a cut would move nothing.
	offered := manager.TurnBoundaries()
	require.Len(t, offered, 3)
	assert.Equal(t, all[:3], offered)
}

// A background delivery wears the user role but is not something the user
// typed, so it is no place to cut the context at.
func TestABackgroundDeliveryIsNoBoundary(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")
	recorded, err := manager.AppendDelivery("watch-1", llm.Message{
		Role:    llm.RoleUser,
		Content: "the build finished",
	})
	require.NoError(t, err)
	require.True(t, recorded)

	for _, boundary := range manager.TurnBoundaries() {
		assert.NotEqual(t, "the build finished", boundary.Prompt)
	}
}

// A prompt carries the harness scaffolding it was sent with. What comes back
// to the composer is what the user typed, not the reminders around it.
func TestAPromptBoundaryDropsTheHarnessScaffolding(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	prompt, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "<system-reminder>\nremember this\n</system-reminder>\nwhat did we decide?",
	})
	require.NoError(t, err)
	_, err = manager.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "we decided"}, "m", "")
	require.NoError(t, err)

	boundary, err := session.TurnBoundaryAt(manager.BuildContext(), prompt)
	require.NoError(t, err)
	assert.Equal(t, "what did we decide?", boundary.Prompt)
}

// A message that is nothing but scaffolding leaves the user nothing to send
// again, so it is not offered as a boundary.
func TestAPromptOfPureScaffoldingIsNoBoundary(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	reminder, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "<system-reminder>\ncontext is filling up\n</system-reminder>",
	})
	require.NoError(t, err)

	_, err = session.TurnBoundaryAt(manager.BuildContext(), reminder)
	var notBoundary *session.NotTurnBoundaryError
	require.ErrorAs(t, err, &notBoundary)
}

// The preview the log yields and the preview a corrected boundary yields are
// built by one rule. A caller that knows better than the record what the user
// typed replaces both the text and the line it is offered under, and the two
// must not drift into showing different things.
func TestBothPreviewPathsUseOneRule(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")

	boundary, err := session.TurnBoundaryAt(manager.BuildContext(), turn.Prompt)
	require.NoError(t, err)
	require.Equal(t, "before one", boundary.Preview)

	assert.Equal(t, boundary.Preview, boundary.WithPrompt(boundary.Prompt).Preview,
		"replacing the prompt with the same text changes nothing")
	assert.Equal(t, "before reworded", boundary.WithPrompt("reworded").Preview)

	answer, err := session.TurnBoundaryAt(manager.BuildContext(), turn.Answer)
	require.NoError(t, err)
	assert.Equal(t, answer, answer.WithPrompt("ignored"),
		"an answer boundary has no prompt to correct")
}
