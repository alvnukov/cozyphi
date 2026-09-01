package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaneJumpSelectsTheBestMatchLive(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(pane, xui.KeyRune, '/', 0))
	require.True(t, pane.State().Jumping)
	paste(pane, "test")
	assert.True(t, selectedRowContains(t, pane, "test-pane"),
		"typing moves the selection to the tightest match")
	assert.Contains(t, renderText(t, pane, 84, 30), "Jump · match 1/")

	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Jumping)
	assert.True(t, selectedRowContains(t, pane, "test-pane"), "Enter keeps what the jump found")
}

func TestPaneJumpEscRestoresTheSelection(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, selectedRowContains(t, pane, "Goal:"))
	require.True(t, key(pane, xui.KeyRune, '/', 0))
	paste(pane, "Add step")
	require.True(t, selectedRowContains(t, pane, "+ Add step"))

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.State().Jumping)
	assert.True(t, selectedRowContains(t, pane, "Goal:"),
		"Esc puts the selection back where the jump started")
}

func TestPaneJumpCyclesMatchesAndNamesAMiss(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(pane, xui.KeyRune, '/', 0))
	paste(pane, "pane")
	first := pane.State().Selected
	require.True(t, key(pane, xui.KeyDown, 0, 0))
	assert.NotEqual(t, first, pane.State().Selected, "↓ moves to the next match")
	require.True(t, key(pane, xui.KeyUp, 0, 0))
	assert.Equal(t, first, pane.State().Selected, "↑ comes back")

	for range 4 { // Erase the query and miss on purpose.
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	paste(pane, "zzqq")
	assert.Contains(t, renderText(t, pane, 84, 30), "Jump · no match")
}

func TestPaneMenuRunsARowCommand(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 9) // First step row.
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	menu := renderText(t, pane, 84, 30)
	assert.Contains(t, menu, "Actions")
	assert.Contains(t, menu, "Open step details (Enter)")
	assert.Contains(t, menu, "Move step down (Alt+↓)")

	selectRow(t, pane, "Move step down")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	after := renderText(t, pane, 84, 30)
	assert.Contains(t, after, "1 ○ run test-pane", "the command ran in the list the menu came from")
	assert.Contains(t, after, "2 unsaved")
	assert.False(t, pane.State().Detail)
}

func TestPaneMenuEscReturnsWithoutActing(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 9)
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.State().Dirty)
	assert.True(t, selectedRowContains(t, pane, "wire the pane"),
		"closing the menu lands back on the row that opened it")
}

func TestPaneMenuOffersUndoOnlyWithHistory(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	assert.NotContains(t, renderText(t, pane, 84, 30), "Undo last edit")
	require.True(t, key(pane, xui.KeyEscape, 0, 0))

	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal editor.
	paste(pane, " v2")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	selectRow(t, pane, "Undo last edit (Ctrl+Z)")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Dirty, "the menu's undo behaves exactly like Ctrl+Z")
}

func TestPaneMenuFromDetailActsOnTheOpenStep(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	openPendingStepDetail(t, pane)
	require.True(t, key(pane, xui.KeyRune, '.', 0))
	menu := renderText(t, pane, 84, 30)
	assert.Contains(t, menu, "Move step up (Alt+↑)")
	assert.Contains(t, menu, "Delete step (Del)")

	selectRow(t, pane, "Move step up")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, pane.State().Detail, "the command runs in the details it came from")
	assert.Contains(t, renderText(t, pane, 84, 30), "Step 1/2 test-pane")
}
