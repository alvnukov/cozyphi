package watch_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// observeSentinel stands in for everything a watch is started with that must
// never reach an answer: it is planted in the label, the command, the match
// expression and the text of the error one of them fails with.
const observeSentinel = "sk-live-OBSERVE-SENTINEL"

// observeShell dispatches on the command so one manager can hold a watch of
// every shape and every ending at once. Nothing here is a real process; what
// matters is which of the manager's paths each command drives it down.
func observeShell(runs chan<- string) watch.ShellFunc {
	return func(ctx context.Context, command string, onChunk func(string)) (watch.ShellResult, error) {
		runs <- command
		switch {
		case strings.Contains(command, "break"):
			return watch.ShellResult{}, errors.New("dial redis://" + observeSentinel + "@cache: refused")
		case strings.Contains(command, "flood"):
			for i := range 64 {
				if ctx.Err() != nil {
					return watch.ShellResult{Canceled: true}, nil
				}
				onChunk(fmt.Sprintf("line %d\n", i))
			}
			return watch.ShellResult{}, nil
		default:
			<-ctx.Done()
			return watch.ShellResult{Canceled: true}, nil
		}
	}
}

// awaitFinal blocks until n watches have published their last event, so an
// observation is taken of a manager that has finished settling.
func awaitFinal(t *testing.T, events <-chan watch.Event, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for n > 0 {
		select {
		case ev := <-events:
			if ev.Final {
				n--
			}
		case <-deadline:
			t.Fatalf("%d watches never finished", n)
		}
	}
}

// A shape is what a watch is, read off the two fields that decide it — and
// for a streaming command the trigger too, because a stream that reports on
// exit stays quiet until the command finishes and quiet is the symptom that
// sends someone here.
func TestObserveReportsEveryShapeAWatchCanTake(t *testing.T) {
	runs := make(chan string, 8)
	mgr := watch.New(watch.Options{Shell: observeShell(runs)})
	t.Cleanup(mgr.Close)

	for _, spec := range []watch.Spec{
		{Label: "app errors", Command: "tail -f app.log", Match: "ERROR"},
		{Label: "the suite", Command: "go test ./internal/...", On: watch.OnExit},
		{Label: "the queue", Command: "queue depth", Every: watch.MinInterval},
		{Label: "stand up", Every: watch.MinInterval},
	} {
		_, err := mgr.Start(spec)
		require.NoError(t, err)
	}

	state := watch.Observe(mgr)

	require.Len(t, state.Watches, 4, "in start order, so an answer can be read against what was started")
	assert.Equal(t, diag.WatchStream, state.Watches[0].Shape)
	assert.Equal(t, diag.WatchOnLine, state.Watches[0].Trigger)
	assert.Equal(t, diag.WatchStream, state.Watches[1].Shape)
	assert.Equal(t, diag.WatchOnExit, state.Watches[1].Trigger)
	assert.Equal(t, diag.WatchPoll, state.Watches[2].Shape)
	assert.Equal(t, watch.MinInterval, state.Watches[2].Every)
	assert.Equal(t, diag.WatchTimer, state.Watches[3].Shape)
	assert.Equal(t, watch.MinInterval, state.Watches[3].Every)

	for i, w := range state.Watches {
		assert.True(t, w.Live, i)
		assert.Equal(t, diag.WatchRunning, w.Outcome, i)
		if w.Shape != diag.WatchStream {
			assert.Empty(t, w.Trigger, "only a streaming command has one")
		}
	}
}

// A watch that failed and a watch that talked itself over the session's event
// budget are different problems with different fixes, and neither is a watch
// that was simply stopped.
func TestObserveTellsTheThreeWaysAWatchStopsApart(t *testing.T) {
	runs := make(chan string, 8)
	mgr := watch.New(watch.Options{Shell: observeShell(runs)})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	broken, err := mgr.Start(watch.Spec{Label: "the deploy", Command: "break the connection"})
	require.NoError(t, err)
	_, err = mgr.Start(watch.Spec{Label: "everything", Command: "flood the session"})
	require.NoError(t, err)
	stopped, err := mgr.Start(watch.Spec{Label: "app errors", Command: "tail -f app.log"})
	require.NoError(t, err)
	awaitFinal(t, events, 2)
	require.NoError(t, mgr.Stop(stopped.ID))
	awaitFinal(t, events, 1)

	state := watch.Observe(mgr)

	require.Len(t, state.Watches, 3)
	assert.Equal(t, diag.WatchFailed, state.Watches[0].Outcome, broken.ID)
	assert.Equal(t, diag.WatchFlooded, state.Watches[1].Outcome)
	assert.Equal(t, diag.WatchEnded, state.Watches[2].Outcome,
		"a watch someone stopped came to no bad end")
	assert.Positive(t, state.Watches[1].Events, "the flood is counted, and only counted")
	for _, w := range state.Watches {
		assert.False(t, w.Live)
	}
}

// A watch is started with a label someone typed, a command someone typed and
// a filter someone typed, and it ends with an error quoting all three. The
// observation is an allowlist of shapes and counts, so none of that has
// anywhere to land.
func TestNothingAWatchWasStartedWithReachesTheObservation(t *testing.T) {
	runs := make(chan string, 8)
	mgr := watch.New(watch.Options{Shell: observeShell(runs)})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	_, err := mgr.Start(watch.Spec{
		Label:   "auth for " + observeSentinel,
		Command: "break: curl -H 'Authorization: Bearer " + observeSentinel + "'",
		Match:   observeSentinel,
	})
	require.NoError(t, err)
	awaitFinal(t, events, 1)

	state := watch.Observe(mgr)

	require.Len(t, state.Watches, 1)
	require.Equal(t, diag.WatchFailed, state.Watches[0].Outcome,
		"the watch did fail, so the error carrying the sentinel exists")
	require.Contains(t, mgr.List()[0].Err, observeSentinel,
		"and the manager still has it, which is why it is worth checking that this does not")
	assert.NotContains(t, fmt.Sprintf("%+v", state), observeSentinel,
		"not in a label, a command, a match expression or an error")
	assert.NotContains(t, state.Revision, observeSentinel)
}

// Reading is reading. Nothing here starts a watch, stops one, subscribes to
// one, waits for one or reads its log — and the shell is the witness, since
// it is the only thing a start would reach.
func TestObservingStartsNoWatchAndStopsNone(t *testing.T) {
	runs := make(chan string, 8)
	mgr := watch.New(watch.Options{Shell: observeShell(runs)})
	t.Cleanup(mgr.Close)

	_, err := mgr.Start(watch.Spec{Label: "app errors", Command: "tail -f app.log"})
	require.NoError(t, err)
	require.Equal(t, "tail -f app.log", <-runs)
	live := mgr.Live()

	for range 5 {
		require.Len(t, watch.Observe(mgr).Watches, 1)
	}

	assert.Empty(t, runs, "no observation ran a command")
	assert.Equal(t, live, mgr.Live(), "and none stopped what was running")
	assert.Len(t, mgr.List(), 1)
}

// The budgets are compiled in, so they hold wherever a manager would be
// built — including a run that never built one. Reporting them from the
// package that enforces them is what keeps the two spellings from drifting.
func TestObserveReportsTheBudgetsTheManagerEnforces(t *testing.T) {
	for name, state := range map[string]diag.WatchState{
		"with a manager": watch.Observe(watch.New(watch.Options{})),
		"without one":    watch.Observe(nil),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, watch.MaxLive, state.MaxLive)
			assert.Equal(t, watch.MinInterval, state.MinInterval)
			assert.Equal(t, watch.FloodLimit, state.FloodLimit)
			assert.Equal(t, time.Minute, state.FloodWindow)
			assert.Equal(t, watch.MaxPerDelivery, state.MaxPerDelivery)
			assert.Equal(t, watch.EventTextLimit, state.EventTextLimit)
		})
	}
}

// A run with no watch manager is not a session that started no watch. The
// first cannot start one at all; the second could at any time.
func TestObserveWithoutAManagerIsNotAnEmptySession(t *testing.T) {
	headless := watch.Observe(nil)
	assert.True(t, headless.Known, "the layer is wired; what is missing is the manager")
	assert.False(t, headless.Managed)
	assert.NotNil(t, headless.Watches, "an empty list, not a nil one")
	assert.Empty(t, headless.Watches)

	mgr := watch.New(watch.Options{})
	t.Cleanup(mgr.Close)
	idle := watch.Observe(mgr)
	assert.True(t, idle.Managed)
	assert.Empty(t, idle.Watches)
	assert.Equal(t, headless.Revision, idle.Revision,
		"the fingerprint describes the watches, and it is the managed flag that tells these apart")
}

// The revision is only good for telling two states apart, and that is what it
// has to do: a snapshot taken across a start must not look like the one taken
// before it.
func TestTheWatchRevisionChangesWhenAWatchStarts(t *testing.T) {
	runs := make(chan string, 8)
	mgr := watch.New(watch.Options{Shell: observeShell(runs)})
	t.Cleanup(mgr.Close)

	before := watch.Observe(mgr).Revision
	_, err := mgr.Start(watch.Spec{Label: "app errors", Command: "tail -f app.log"})
	require.NoError(t, err)

	assert.NotEqual(t, before, watch.Observe(mgr).Revision)
}
