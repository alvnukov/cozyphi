package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/watch"
)

// blockingShell streams one line and then holds the watch open until it is
// stopped: a watch that is predictably live, which is what these tests need.
func blockingShell(line string) watch.ShellFunc {
	return func(ctx context.Context, command string, onChunk func(string)) (watch.ShellResult, error) {
		onChunk(line + "\n")
		<-ctx.Done()
		return watch.ShellResult{Canceled: true}, nil
	}
}

// newWatchController builds the smallest controller the watch read seam needs:
// just the manager. The wake machinery is orthogonal and covered elsewhere.
func newWatchController(t *testing.T, shell watch.ShellFunc) (*Controller, watch.Watch) {
	m := watch.New(watch.Options{Shell: shell, Now: func() time.Time { return time.Now() }})
	c := &Controller{watches: m}
	w, err := m.Start(watch.Spec{Label: "edge logs", Command: "tail -f app.log"})
	require.NoError(t, err)
	return c, w
}

// TestWatchSeamExposesTheManager is the pane/footer contract in one test: the
// controller hands a dumb widget the list, the log and the stop button — the
// widget never touches the manager itself.
func TestWatchSeamExposesTheManager(t *testing.T) {
	c, w := newWatchController(t, blockingShell("ERROR edge refused"))

	waitForCond(t, 5*time.Second, func() bool {
		for _, item := range c.WatchList() {
			if item.ID == w.ID && item.Live {
				return true
			}
		}
		return false
	})

	waitForCond(t, 5*time.Second, func() bool {
		log, err := c.WatchLog(w.ID, 10)
		return err == nil && len(log) > 0 && log[0].Text == "ERROR edge refused"
	})

	require.NoError(t, c.StopWatch(w.ID))
	waitForCond(t, 5*time.Second, func() bool {
		for _, item := range c.WatchList() {
			if item.ID == w.ID {
				return !item.Live
			}
		}
		return false
	})
}

// TestWatchSeamReportsUnknownIDs pins the failure shape: a stale row in the
// pane (the watch ended and was pruned between draws) says "no such watch",
// not panic and not silence.
func TestWatchSeamReportsUnknownIDs(t *testing.T) {
	c, _ := newWatchController(t, blockingShell("irrelevant"))

	_, err := c.WatchLog("no-such-watch", 10)
	assert.ErrorIs(t, err, watch.ErrNotFound)
	assert.ErrorIs(t, c.StopWatch("no-such-watch"), watch.ErrNotFound)
}

// TestWatchSeamWithoutManager mirrors LiveJobCount: a controller that has no
// watch manager (tests, sub-agents) reports emptiness instead of crashing.
func TestWatchSeamWithoutManager(t *testing.T) {
	c := &Controller{}
	assert.Empty(t, c.WatchList())
	_, err := c.WatchLog("any", 10)
	assert.Error(t, err)
	assert.Error(t, c.StopWatch("any"))

	var nilCtrl *Controller
	assert.Empty(t, nilCtrl.WatchList())
	assert.Error(t, nilCtrl.StopWatch("any"))
}
