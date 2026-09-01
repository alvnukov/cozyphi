package planedit_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaneUndoRedoWalkAFieldEdit(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})

	key(pane, xui.KeyEnter, 0, 0) // Goal popup.
	paste(pane, " v2")
	key(pane, xui.KeyEnter, 0, 0)
	require.True(t, pane.State().Dirty)
	text := renderText(t, pane, 100, 30)
	assert.Contains(t, text, "● Goal")
	assert.Contains(t, text, "1 unsaved")

	key(pane, xui.KeyRune, 'z', xui.ModCtrl)
	assert.False(t, pane.State().Dirty)
	text = renderText(t, pane, 100, 30)
	assert.NotContains(t, text, "●")
	assert.NotContains(t, text, "unsaved")
	assert.NotContains(t, text, "v2")

	key(pane, xui.KeyRune, 'y', xui.ModCtrl)
	assert.True(t, pane.State().Dirty)
	text = renderText(t, pane, 100, 30)
	assert.Contains(t, text, "● Goal")
	assert.Contains(t, text, "v2")
}

func TestPaneUndoWithNothingRecordedSaysSo(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	key(pane, xui.KeyRune, 'z', xui.ModCtrl)
	assert.Equal(t, "nothing to undo", pane.State().Error)
	key(pane, xui.KeyRune, 'y', xui.ModCtrl)
	assert.Equal(t, "nothing to redo", pane.State().Error)
}

func TestPaneUndoRestoresADeletedStep(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 10) // The pending second step.
	key(pane, xui.KeyDelete, 0, 0)
	require.True(t, pane.State().Confirming)
	key(pane, xui.KeyRune, 'y', 0)
	require.True(t, pane.State().Dirty)
	assert.NotContains(t, renderText(t, pane, 100, 30), "test-pane")

	key(pane, xui.KeyRune, 'z', xui.ModCtrl)
	assert.False(t, pane.State().Dirty)
	assert.Contains(t, renderText(t, pane, 100, 30), "test-pane")
}

func TestPaneReorderMarksBothMovedSteps(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 9) // First step row.
	key(pane, xui.KeyDown, 0, xui.ModAlt)
	text := renderText(t, pane, 100, 30)
	assert.Equal(t, 2, strings.Count(text, "●"))
	assert.Contains(t, text, "2 unsaved")

	key(pane, xui.KeyRune, 'z', xui.ModCtrl)
	assert.Zero(t, strings.Count(renderText(t, pane, 100, 30), "●"))
	assert.False(t, pane.State().Dirty)
}

func TestPaneHeaderCountsUnsavedEdits(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	key(pane, xui.KeyEnter, 0, 0) // Goal popup.
	paste(pane, " v2")
	key(pane, xui.KeyEnter, 0, 0)
	down(t, pane, 5) // + Add success criterion.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "fresh criterion")
	key(pane, xui.KeyEnter, 0, 0)

	text := renderText(t, pane, 100, 30)
	assert.Contains(t, text, "2 unsaved")
	assert.Equal(t, 2, strings.Count(text, "●"))
}

func TestPaneDetailMarksTheEditedFieldOnly(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 9)
	key(pane, xui.KeyEnter, 0, 0) // Step details; the cursor rests on Content.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " now")
	key(pane, xui.KeyEnter, 0, 0)
	// 84 renders the details alone; a wide screen would also show the step's
	// own marker in the master list.
	text := renderText(t, pane, 84, 30)
	assert.Contains(t, text, "● Content")
	assert.Equal(t, 1, strings.Count(text, "●"))
	assert.Contains(t, text, "1 unsaved")

	key(pane, xui.KeyRune, 'z', xui.ModCtrl) // Undo works from the details too.
	assert.Zero(t, strings.Count(renderText(t, pane, 84, 30), "●"))
	assert.False(t, pane.State().Dirty)
	assert.True(t, pane.State().Detail)
}

func TestPaneUndoIsRefusedInsideAChoiceList(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan(), models: []string{"plan-a"}})
	key(pane, xui.KeyEnter, 0, 0) // Goal popup: give the history one edit.
	paste(pane, " v2")
	key(pane, xui.KeyEnter, 0, 0)

	key(pane, xui.KeyEnd, 0, 0)   // The last row is a model pin.
	key(pane, xui.KeyEnter, 0, 0) // Its choice list.
	key(pane, xui.KeyRune, 'z', xui.ModCtrl)
	assert.Contains(t, pane.State().Error, "finish the choice")

	key(pane, xui.KeyEscape, 0, 0)
	assert.True(t, pane.State().Dirty, "the refused undo must not have eaten the edit")
}
