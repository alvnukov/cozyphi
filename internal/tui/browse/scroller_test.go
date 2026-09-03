package browse_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

func TestScrollerAppliesMotions(t *testing.T) {
	var s browse.Scroller
	s.SetExtent(100, 24)

	s.Apply(browse.Motion{Op: browse.OpStep, N: 5})
	assert.Equal(t, 5, s.Offset())

	s.Apply(browse.Motion{Op: browse.OpPage, N: 1})
	assert.Equal(t, 5+23, s.Offset(), "a page keeps one line of overlap")

	s.Apply(browse.Motion{Op: browse.OpHalfPage, N: -1})
	assert.Equal(t, 5+23-12, s.Offset())

	s.Apply(browse.Motion{Op: browse.OpBottom})
	assert.Equal(t, 76, s.Offset(), "the last screen sits flush with the bottom")

	s.Apply(browse.Motion{Op: browse.OpTop})
	assert.Zero(t, s.Offset())

	s.Apply(browse.Motion{Op: browse.OpIndex, N: 50})
	assert.Equal(t, 49, s.Offset(), "50G puts line 50 first")

	s.Apply(browse.Motion{Op: browse.OpNone, N: 9})
	assert.Equal(t, 49, s.Offset(), "a pending keypress moves nothing")
}

func TestScrollerClampsJumpsAndShrinks(t *testing.T) {
	var s browse.Scroller
	s.SetExtent(100, 24)

	s.Jump(1000)
	assert.Equal(t, 76, s.Offset())
	s.Jump(-5)
	assert.Zero(t, s.Offset())

	s.Jump(76)
	s.SetExtent(30, 24)
	assert.Equal(t, 6, s.Offset(), "shrinking content pulls the window back in")

	s.SetExtent(10, 24)
	assert.Zero(t, s.Offset(), "content shorter than the window pins to the top")
}

func TestScrollerSurvivesAnUnmeasuredViewport(t *testing.T) {
	// Keys can arrive before the first Draw measures anything; the window
	// must stay inside the content it knows about.
	var s browse.Scroller
	s.Apply(browse.Motion{Op: browse.OpPage, N: 1})
	assert.Zero(t, s.Offset(), "an empty scroller has nowhere to go")

	s.SetExtent(10, 0)
	s.Apply(browse.Motion{Op: browse.OpPage, N: 1})
	assert.Equal(t, 1, s.Offset(), "a zero viewport still pages a line at a time")
}
