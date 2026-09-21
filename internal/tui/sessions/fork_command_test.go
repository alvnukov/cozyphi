package sessions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The session registers /fork, and a fork asked for reaches an answer rather
// than a shrug. Before the copy was implemented the command did not exist and
// the composer sent the line off as a prompt.
func TestSlashForkIsRegisteredAndAnswers(t *testing.T) {
	e := newActionsEditor(t)

	require.True(t, e.commands.DispatchSlash("/fork", e.commandContext()),
		"/fork is a command of this session")

	history := e.toast.History()
	require.NotEmpty(t, history, "the fork answered")
	last := history[len(history)-1].Message
	assert.NotContains(t, last, "not wired up yet",
		"the fork is an operation now, not a placeholder")
}
