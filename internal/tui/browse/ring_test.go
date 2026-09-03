package browse_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

func TestRingWrapsAtBothEdges(t *testing.T) {
	var r browse.Ring
	r.SetLen(4)

	r.Step(-1)
	assert.Equal(t, 3, r.Selected(), "stepping up from the top lands on the bottom")
	r.Step(1)
	assert.Equal(t, 0, r.Selected(), "stepping down from the bottom lands on the top")
	r.Step(2)
	assert.Equal(t, 2, r.Selected())
}

func TestRingKeepsTheSelectionInsideARebuiltList(t *testing.T) {
	var r browse.Ring
	r.SetLen(5)
	r.Select(4)

	r.SetLen(2)
	assert.Equal(t, 1, r.Selected(), "a shrunken list pulls the selection to its last option")

	r.SetLen(0)
	assert.Zero(t, r.Selected())
	r.Step(1)
	assert.Zero(t, r.Selected(), "an empty ring has nowhere to go")

	r.Select(7)
	assert.Zero(t, r.Selected(), "selecting into an empty ring stays at zero")
}
