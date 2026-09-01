package planedit_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two-pane layout switches on at panel width twoPaneMinPanel; a terminal
// width of 100 is comfortably above it, 84 comfortably below.

func TestPaneWideScreenPreviewsTheSelection(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})

	// The goal row is selected on entry; the preview shows the field in full
	// with its length against the limit.
	wide := renderText(t, pane, 100, 30)
	assert.Contains(t, wide, "Goal · 20/512")

	// A step row previews the step's detail form without opening it.
	selectRow(t, pane, "wire the pane")
	wide = renderText(t, pane, 100, 30)
	assert.Contains(t, wide, "Done when: the modal opens")
	assert.False(t, pane.State().Detail, "the preview is not the detail view")

	// Rows with nothing of their own fall back to the plan overview.
	selectRow(t, pane, "+ Add step")
	assert.Contains(t, renderText(t, pane, 100, 30), "2 steps · 0 done")
}

func TestPaneNarrowScreenStaysSingleList(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	narrow := renderText(t, pane, 84, 30)
	assert.NotContains(t, narrow, "Goal · 20/512", "no preview column below the threshold")
	assert.Equal(t, 1, strings.Count(narrow, "›"), "one list, one selection marker")
}

func TestPaneDetailKeepsTheMasterListVisible(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	openPendingStepDetail(t, pane)

	wide := renderText(t, pane, 100, 30)
	assert.Contains(t, wide, "Step 2/2 test-pane", "the detail form fills the right pane")
	assert.Contains(t, wide, "Success criteria", "the plan list stays on the left")
	assert.Equal(t, 2, strings.Count(wide, "›"),
		"the focused detail row and the passive master row are both marked")

	narrow := renderText(t, pane, 84, 30)
	assert.NotContains(t, narrow, "Success criteria", "below the threshold the detail stands alone")
	assert.Equal(t, 1, strings.Count(narrow, "›"))
}

func TestPaneEscFromDetailLandsOnTheStepRow(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	openPendingStepDetail(t, pane)
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.State().Detail)
	assert.True(t, selectedRowContains(t, pane, "test the pane"),
		"closing the detail parks the selection on the step it showed")
}

func TestPaneChoiceRoundTripKeepsTheDetailSelection(t *testing.T) {
	pane := newPane(actionStore())
	openPendingStepDetail(t, pane)

	// Backing out of the model list returns to the same detail row.
	selectRow(t, pane, "Model:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.True(t, pane.State().Detail)
	assert.True(t, selectedRowContains(t, pane, "Model:"))

	// So does picking a value.
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "plan-b")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, pane.State().Detail)
	assert.True(t, selectedRowContains(t, pane, "Model: plan-b"))
}
