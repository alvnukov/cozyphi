package watchpane

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// fixtureWatches is a session's watch history: one running, one ended
// cleanly, one that failed — the three states the pane must tell apart.
func fixtureWatches() []watch.Watch {
	return []watch.Watch{
		{ID: "w1", Label: "edge logs", Live: true, Started: time.Now().Add(-3 * time.Minute), Events: 12},
		{ID: "w2", Label: "build", Started: time.Now().Add(-12 * time.Second)},
		{ID: "w3", Label: "deploy", Started: time.Now().Add(-time.Minute), Events: 2, Err: "exit 1"},
	}
}

type harness struct {
	pane    *Pane
	stopped *[]string
	closed  *int
}

func newHarness(ws []watch.Watch, events []watch.Event) *harness {
	var stopped []string
	closed := 0
	p := New(
		components.DefaultTheme(),
		func() []watch.Watch { return ws },
		func(_ string, _ int) ([]watch.Event, error) { return events, nil },
		func(id string) error { stopped = append(stopped, id); return nil },
		func() { closed++ },
	)
	return &harness{pane: p, stopped: &stopped, closed: &closed}
}

func press(t *testing.T, p *Pane, code xui.KeyCode, r rune) bool {
	t.Helper()
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func drawText(t *testing.T, p *Pane) string {
	t.Helper()
	return components.SurfaceText(p.Draw(components.DrawContext{
		Max:    components.Size{Width: 80, Height: 24},
		Method: xui.WidthUnicode,
	}))
}

// The list names each watch's state, label, event count and age — the same
// facts watch action=list gives the model, on screen.
func TestPaneDrawsWatchStates(t *testing.T) {
	h := newHarness(fixtureWatches(), nil)
	h.pane.Show()

	text := drawText(t, h.pane)
	assert.Contains(t, text, "Watches")
	assert.Contains(t, text, "running")
	assert.Contains(t, text, "edge logs")
	assert.Contains(t, text, "ended")
	assert.Contains(t, text, "build")
	assert.Contains(t, text, "failed")
	assert.Contains(t, text, "deploy")
	assert.Contains(t, text, "12")
}

// Enter opens the selected watch's log over the list; Escape returns to the
// list without closing the pane.
func TestPaneEnterOpensLogPopup(t *testing.T) {
	events := []watch.Event{
		{ID: "w1", Label: "edge logs", Text: "ERROR edge refused"},
		{ID: "w1", Label: "edge logs", Text: "ERROR edge timed out", Final: true},
	}
	h := newHarness(fixtureWatches(), events)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyEnter, 0))
	require.True(t, h.pane.popup)
	assert.Contains(t, drawText(t, h.pane), "ERROR edge refused")

	require.True(t, press(t, h.pane, xui.KeyEscape, 0))
	assert.False(t, h.pane.popup)
	assert.True(t, h.pane.Visible(), "Escape in the popup closes only the popup")
}

// Stopping asks first: s arms a y/n confirmation naming the watch, y stops
// it through the seam, n withdraws the question.
func TestPaneStopAsksBeforeStopping(t *testing.T) {
	h := newHarness(fixtureWatches(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyRune, 's'))
	require.True(t, h.pane.confirm.Armed())
	assert.Empty(t, *h.stopped, "s alone must not stop anything")
	assert.Contains(t, drawText(t, h.pane), "stop watch")

	require.True(t, press(t, h.pane, xui.KeyRune, 'y'))
	assert.Equal(t, []string{"w1"}, *h.stopped)
	assert.False(t, h.pane.confirm.Armed())
}

// A dead watch cannot be stopped: s says so instead of posing the question.
func TestPaneStopOnEndedWatchSaysSo(t *testing.T) {
	h := newHarness(fixtureWatches(), nil)
	h.pane.Show()
	h.pane.cursor.Select(1) // "build", ended

	require.True(t, press(t, h.pane, xui.KeyRune, 's'))
	assert.False(t, h.pane.confirm.Armed())
	assert.Empty(t, *h.stopped)
	assert.Contains(t, drawText(t, h.pane), "already ended")
}

// Escape closes the pane and notifies the owner exactly once, no matter how
// often Hide lands.
func TestPaneCloseNotifiesOwnerOnce(t *testing.T) {
	h := newHarness(fixtureWatches(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyEscape, 0))
	assert.False(t, h.pane.Visible())
	assert.Equal(t, 1, *h.closed)

	h.pane.Hide()
	assert.Equal(t, 1, *h.closed, "a second Hide stays silent")
}

// A session with no watches opens the pane just fine: an empty note, no
// crash, keys dead.
func TestPaneEmptySession(t *testing.T) {
	h := newHarness(nil, nil)
	h.pane.Show()

	assert.Contains(t, drawText(t, h.pane), "no watches")
	require.True(t, press(t, h.pane, xui.KeyEnter, 0))
	assert.False(t, h.pane.popup)
}
