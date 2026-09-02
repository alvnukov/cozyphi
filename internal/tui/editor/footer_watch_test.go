package editor

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// A left click on the footer's watch label folds or unfolds that watch's
// rows in the feed, and a second click undoes it. Off the indicator the
// footer stays inert, and a watch with no rows in the feed says so in a
// toast rather than swallowing the click.
func TestEditorFooterClickFoldsTheWatchRows(t *testing.T) {
	e := newTestEditor(t)
	live := []watch.Watch{{ID: "w1", Label: "edge logs", Live: true}}
	e.footer.SetLiveWatches(func() []watch.Watch { return live })
	e.transcript.ApplySession(session.WatchFired{ID: "w1-1", Label: "edge logs", Text: "hit: line one"})
	e.transcript.Sync()

	draw := func() components.Surface {
		return e.Draw(components.DrawContext{Max: components.Size{Width: 120, Height: 40}, Method: xui.WidthUnicode})
	}
	feedShows := func(root components.Surface) bool {
		return strings.Contains(components.SurfaceText(root.Children[0].Surface), "hit: line one")
	}
	root := draw()
	require.Equal(t, 39, e.footerY, "the footer sits on the last row")
	require.Contains(t, components.SurfaceText(root.Children[2].Surface), "⏱ 1 watch: edge logs")
	before := feedShows(root)

	x := -1
	for col := range 120 {
		if _, ok := e.footer.WatchesAt(col); ok {
			x = col
			break
		}
	}
	require.NotEqual(t, -1, x, "the indicator must be clickable")

	click := func(col, row int) *components.EventContext {
		ctx := &components.EventContext{}
		e.Handle(ctx, xui.MouseEvent{X: col, Y: row, Button: xui.MouseLeft, Action: xui.MousePress})
		return ctx
	}
	require.True(t, click(x, e.footerY).Consume, "the indicator takes the click")
	require.NotEqual(t, before, feedShows(draw()), "the click flips the watch's rows")
	require.True(t, click(x, e.footerY).Consume)
	require.Equal(t, before, feedShows(draw()), "the second click flips them back")

	require.False(t, click(x, e.footerY-1).Consume, "a row above the footer is not the indicator")
	require.False(t, e.toast.Visible())

	// A watch that is live but has left no rows in the feed.
	live = []watch.Watch{{ID: "w2", Label: "build", Live: true}}
	draw()
	require.True(t, click(x, e.footerY).Consume)
	require.Contains(t, e.toast.Message, "No transcript rows for w2")
}
