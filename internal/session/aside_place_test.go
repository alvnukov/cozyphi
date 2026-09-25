package session_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// Each side question takes its place after the leaf it was asked at, however
// many turns come after it, and keeps naming what it was about.
func TestPathAsidesFollowTheLeafTheyWereAskedAt(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")
	whole := askAside(t, manager, "", "about everything?", "yes")
	earlier := askAside(t, manager, first.Answer, "about the first?", "that one")
	recordTurn(t, manager, "three", "answer three")

	placed := manager.PathAsides()
	require.Len(t, placed, 2)
	assert.Equal(t, whole.ID, placed[0].ID)
	assert.Equal(t, second.Answer, placed[0].After)
	assert.Empty(t, placed[0].AnchorPreview)
	assert.Equal(t, earlier.ID, placed[1].ID)
	assert.Equal(t, second.Answer, placed[1].After)
	assert.Equal(t, anchorPreviewOf(t, manager, first.Answer), placed[1].AnchorPreview)
	assert.Equal(t, "that one", placed[1].Answer)
}

// A question asked on a branch the cursor has left is not shown: the feed no
// longer holds what it was about. It is not lost either, and comes back with
// the branch.
func TestPathAsidesLeaveOutABranchTheCursorLeft(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")
	askAside(t, manager, "", "about two?", "yes")

	_, err := manager.Rewind(second.Prompt)
	require.NoError(t, err)
	assert.Empty(t, manager.PathAsides(), "the aside was asked on the branch the rewind left")
	assert.Len(t, manager.Asides(), 1, "the record itself stays")

	onNewBranch := askAside(t, manager, "", "about one?", "sure")
	placed := manager.PathAsides()
	require.Len(t, placed, 1)
	assert.Equal(t, onNewBranch.ID, placed[0].ID)
	assert.Equal(t, first.Answer, placed[0].After)

	// The leaf the new question was asked at lies on the old branch too, so
	// back there both questions show, each after its own leaf.
	_, err = manager.UndoRewind()
	require.NoError(t, err)
	placed = manager.PathAsides()
	require.Len(t, placed, 2)
	assert.Equal(t, "about two?", placed[0].Question, "back on the old branch, its aside is back")
	assert.Equal(t, second.Answer, placed[0].After)
	assert.Equal(t, first.Answer, placed[1].After)
}
