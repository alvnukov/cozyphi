package footer

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
