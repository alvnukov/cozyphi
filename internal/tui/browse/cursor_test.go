package browse_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

// sparse is a 10-row list where only rows 1, 4 and 7 hold the selection —
// the shape of a pane with section headers and spacers.
func sparse() *browse.Cursor {
	var c browse.Cursor
	c.SetRows(10, func(i int) bool { return i%3 == 1 })
	c.SetViewport(10)
	return &c
}

func TestCursorNeverRestsOnASpacer(t *testing.T) {
	c := sparse()
	assert.Equal(t, 1, c.Selected(), "row 0 is a header; the cursor opens below it")

	c.MoveBy(1)
	assert.Equal(t, 4, c.Selected(), "one step crosses the spacers")

	c.MoveBy(10)
	assert.Equal(t, 7, c.Selected(), "steps stop at the last selectable row")

	c.MoveBy(-1)
	assert.Equal(t, 4, c.Selected())

	c.Apply(browse.Motion{Op: browse.OpBottom})
	assert.Equal(t, 7, c.Selected(), "the bottom is the last selectable row, not row 9")

	c.Apply(browse.Motion{Op: browse.OpTop})
	assert.Equal(t, 1, c.Selected())

	c.Apply(browse.Motion{Op: browse.OpIndex, N: 100})
	assert.Equal(t, 7, c.Selected(), "an index past the end snaps back to a selectable row")
}

func TestCursorCountsSelectableSteps(t *testing.T) {
	c := sparse()
	c.Apply(browse.Motion{Op: browse.OpStep, N: 2})
	assert.Equal(t, 7, c.Selected(), "2j is two rows a user can see, not two array slots")
}

func TestCursorWindowFollowsTheSelection(t *testing.T) {
	var c browse.Cursor
	c.SetRows(30, nil)
	c.SetViewport(5)

	c.Select(10)
	assert.Equal(t, 10, c.Selected())
	assert.Equal(t, 6, c.Scroll(), "the window slides down just enough")

	c.Apply(browse.Motion{Op: browse.OpTop})
	assert.Zero(t, c.Scroll())

	c.Apply(browse.Motion{Op: browse.OpPage, N: 1})
	assert.Equal(t, 4, c.Selected(), "a page keeps one row of overlap")
}

func TestCursorWheelLeavesTheSelection(t *testing.T) {
	var c browse.Cursor
	c.SetRows(30, nil)
	c.SetViewport(5)

	assert.True(t, c.Wheel(xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: 2}))
	assert.Equal(t, 6, c.Scroll())
	assert.Zero(t, c.Selected(), "the wheel moves the window, not the cursor")

	assert.False(t, c.Wheel(xui.MouseEvent{Button: xui.MouseLeft}))
}

func TestCursorSurvivesRebuiltRows(t *testing.T) {
	var c browse.Cursor
	c.SetRows(20, nil)
	c.SetViewport(5)
	c.Select(15)

	c.SetRows(4, nil)
	assert.Equal(t, 3, c.Selected(), "a shrunken list pulls the cursor back in")
	assert.LessOrEqual(t, c.Scroll(), c.Selected())

	c.SetRows(0, nil)
	assert.Zero(t, c.Selected(), "an empty list parks the cursor at zero")
	assert.Zero(t, c.Scroll())
}
