package footer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// watchFooter pairs a chrome with a fixed watch snapshot, the way the editor
// will wire it to controller.WatchList.
func watchFooter(ws []watch.Watch) *FooterChrome {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetLiveWatches(func() []watch.Watch { return ws })
	return f
}

// The quiet footer names the live watches — count plus labels — so a watch
// running in the background is visible from the first row, not just in the
// transcript.
func TestQuietFooterCountsLiveWatches(t *testing.T) {
	f := watchFooter([]watch.Watch{
		{ID: "w1", Label: "edge logs", Live: true},
		{ID: "w2", Label: "build", Live: true},
	})
	assert.Contains(t, drawRow(f, 80), "⏱ 2 watches: edge logs, build")
}

// The indicator counts live watches only: finished ones are history the
// transcript already shows, and zero live watches keeps the row as it was.
func TestQuietFooterHidesTheWatchIndicatorWhenNoneIsLive(t *testing.T) {
	f := watchFooter([]watch.Watch{{ID: "w1", Label: "edge logs"}})
	assert.NotContains(t, drawRow(f, 80), "⏱")

	bare := NewFooterChrome(components.DefaultTheme(), 0)
	bare.SetLiveWatches(nil)
	assert.NotContains(t, drawRow(bare, 80), "⏱")
}

// The streaming footer carries the same indicator, so a watch stays visible
// while the model works.
func TestLiveFooterCountsLiveWatches(t *testing.T) {
	f := watchFooter([]watch.Watch{{ID: "w1", Label: "edge logs", Live: true}})
	snap := liveSnap()
	f.SetLabelContext(func() session.Snapshot { return snap })
	f.Activity().Apply(controller.ActivityStreaming)

	assert.Contains(t, drawRow(f, 80), "⏱ 1 watch: edge logs")
}

// A click on the indicator is read back by column: a label names its watch,
// the glyph and the count name every live one, and anything off the
// indicator — the pad column, the empty tail — names nothing. The pointer
// shape follows the same map.
func TestWatchIndicatorMapsClickColumnsToWatches(t *testing.T) {
	f := watchFooter([]watch.Watch{
		{ID: "w1", Label: "edge logs", Live: true},
		{ID: "w2", Label: "build", Live: true},
	})
	drawRow(f, 80)
	require.NotEmpty(t, f.hits, "the indicator must record its columns")

	var head, second watchHit
	for _, h := range f.hits {
		switch {
		case h.watch == "" && head.x1 == 0:
			head = h
		case h.watch == "w2":
			second = h
		}
	}
	require.NotZero(t, second.x1, "the second label must be a click target")

	got, ok := f.WatchesAt(second.x0)
	require.True(t, ok)
	assert.Equal(t, []watch.Watch{{ID: "w2", Label: "build", Live: true}}, got)
	assert.Equal(t, components.ShapePointer, f.pointer.PointerShape(second.x0, 0))

	got, ok = f.WatchesAt(head.x0)
	require.True(t, ok, "the glyph addresses every live watch")
	assert.Len(t, got, 2)

	_, ok = f.WatchesAt(0)
	assert.False(t, ok, "the pad column is not the indicator")
	assert.Empty(t, f.pointer.PointerShape(0, 0))
	_, ok = f.WatchesAt(79)
	assert.False(t, ok, "the empty tail is not the indicator")
}

// The columns are those of the last frame; a watch that ended since then
// leaves its label's click with nothing to act on, and the indicator only
// asks for frames while something is live.
func TestWatchIndicatorForgetsAnEndedWatch(t *testing.T) {
	ws := []watch.Watch{{ID: "w1", Label: "edge logs", Live: true}}
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetLiveWatches(func() []watch.Watch { return ws })
	assert.True(t, f.WatchesLive())
	drawRow(f, 80)
	label := f.hits[len(f.hits)-1]
	require.Equal(t, "w1", label.watch)
	_, ok := f.WatchesAt(label.x0)
	require.True(t, ok)

	ws[0].Live = false
	assert.False(t, f.WatchesLive())
	_, ok = f.WatchesAt(label.x0)
	assert.False(t, ok, "an ended watch is no longer a target")
	drawRow(f, 80)
	assert.Empty(t, f.hits, "the next frame drops the indicator")
}

// A watch started without a label still gets a clickable name.
func TestWatchIndicatorNamesAnUnlabeledWatch(t *testing.T) {
	f := watchFooter([]watch.Watch{{ID: "w1", Live: true}})
	assert.Contains(t, drawRow(f, 80), "⏱ 1 watch: (unlabeled)")
}
