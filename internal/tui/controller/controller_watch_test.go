package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// textSSEServer answers every request with one line of assistant text and
// records the request bodies, which is where a woken turn's prompt shows up.
func textSSEServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu       sync.Mutex
		recorded []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"noted\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
}

func watchEvent(label, text string) watch.Event {
	return watch.Event{ID: "w1", Label: label, Text: text, Time: time.Now()}
}

// TestWatchEventWakesAnIdleSession is the whole feature in one test: nobody is
// typing, nothing is running, a watch fires — and the agent gets a turn about
// it without anyone asking.
func TestWatchEventWakesAnIdleSession(t *testing.T) {
	srv, bodies := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	ctrl.observeWatchEvent(watchEvent("errors in deploy.log", "ERROR connection refused"))

	// The user sees it at once, before any model round trip.
	var fired session.WatchFired
	waitForCond(t, 5*time.Second, func() bool {
		for _, msg := range bus.Drain() {
			if ev, ok := msg.(SessionEventMsg); ok {
				if wf, ok := ev.Event.(session.WatchFired); ok {
					fired = wf
					return true
				}
			}
		}
		return false
	})
	assert.Equal(t, "errors in deploy.log", fired.Label)
	assert.Equal(t, "ERROR connection refused", fired.Text)

	waitForCond(t, 10*time.Second, func() bool { return len(bodies()) >= 1 })
	body := bodies()[0]
	assert.Contains(t, body, "ERROR connection refused", "the model is told what happened")
	assert.Contains(t, body, "system-reminder", "and told it did not come from the user")
	assert.Contains(t, body, "not a\\nmessage from the user")
}

// TestABurstOfEventsBecomesOneTurn pins the coalescing window: five things
// happening at once are one thing to wake up for, not five.
func TestABurstOfEventsBecomesOneTurn(t *testing.T) {
	srv, bodies := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	for i := range 5 {
		ctrl.observeWatchEvent(watchEvent("log", fmt.Sprintf("ERROR %d", i)))
	}

	waitForCond(t, 10*time.Second, func() bool { return len(bodies()) >= 1 })
	// Give a second turn every chance to start before ruling it out.
	time.Sleep(2 * watchWakeDelay)
	got := bodies()
	require.Len(t, got, 1, "one burst, one turn")
	assert.Contains(t, got[0], "ERROR 0")
	assert.Contains(t, got[0], "ERROR 4", "every event in the burst travels with it")
}

// TestWakeStreakStopsARunawayWatch pins the brake on the obvious failure: a
// watch whose events are caused by the turn they woke. Past the streak the
// events keep arriving and keep showing, but they stop starting turns.
func TestWakeStreakStopsARunawayWatch(t *testing.T) {
	srv, _ := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	ctrl.streamMu.Lock()
	ctrl.wakeStreak = maxWakeStreak
	ctrl.streamMu.Unlock()

	ctrl.observeWatchEvent(watchEvent("loop", "again"))

	ctrl.streamMu.Lock()
	armed := ctrl.watchWake != nil
	queued := len(ctrl.watchQueue)
	ctrl.streamMu.Unlock()
	assert.False(t, armed, "a spent streak arms no wake")
	assert.Equal(t, 1, queued, "but the event is kept, not dropped")

	// The user saying anything at all lifts the brake.
	ctrl.StartPrompt("what happened?", nil, "u1")
	ctrl.streamMu.Lock()
	streak := ctrl.wakeStreak
	remaining := len(ctrl.watchQueue)
	ctrl.streamMu.Unlock()
	assert.Equal(t, 0, streak)
	assert.Equal(t, 0, remaining, "and the waiting events ride along with the prompt")
}

// TestEscCallsOffAPendingWake pins what Esc means with nothing running: do not
// start that turn. The events are not lost — the next prompt carries them.
func TestEscCallsOffAPendingWake(t *testing.T) {
	srv, bodies := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	ctrl.observeWatchEvent(watchEvent("log", "ERROR later"))
	ctrl.streamMu.Lock()
	armed := ctrl.watchWake != nil
	ctrl.streamMu.Unlock()
	require.True(t, armed, "an idle session arms a wake")

	ctrl.Cancel()

	ctrl.streamMu.Lock()
	stillArmed := ctrl.watchWake != nil
	queued := len(ctrl.watchQueue)
	ctrl.streamMu.Unlock()
	assert.False(t, stillArmed)
	assert.Equal(t, 1, queued)

	time.Sleep(3 * watchWakeDelay)
	assert.Empty(t, bodies(), "Esc means no turn starts")

	ctrl.StartPrompt("anything new?", nil, "u1")
	waitForCond(t, 10*time.Second, func() bool { return len(bodies()) >= 1 })
	assert.Contains(t, bodies()[0], "ERROR later", "the event waited for the user instead")
	assert.Contains(t, bodies()[0], "anything new?")
}

// TestWatchEventRidesIntoARunningTurn pins the other delivery path: with a
// turn already running there is nothing to wake, so the event joins that turn
// rather than queuing for the next one.
func TestWatchEventRidesIntoARunningTurn(t *testing.T) {
	srv, _ := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamMu.Unlock()

	ctrl.observeWatchEvent(watchEvent("ci", "checks passed"))

	ctrl.streamMu.Lock()
	armed := ctrl.watchWake != nil
	queued := len(ctrl.watchQueue)
	ctrl.streamMu.Unlock()
	assert.False(t, armed, "a running turn needs no wake")
	assert.Equal(t, 1, queued, "the running turn drains it at its next tool round")

	ctrl.streamMu.Lock()
	drained := ctrl.drainWatchLocked()
	ctrl.streamRunning = false
	ctrl.streamMu.Unlock()
	require.Len(t, drained, 1)
	assert.Equal(t, "checks passed", drained[0].Text)
}

// TestTheQueueDropsTheOldestUnderPressure pins the bound: a watch nobody reads
// must not grow the queue without limit.
func TestTheQueueDropsTheOldestUnderPressure(t *testing.T) {
	srv, _ := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = true // nothing drains while a turn is "running"
	ctrl.streamMu.Unlock()

	for i := range watchQueueLimit + 10 {
		ctrl.observeWatchEvent(watchEvent("noisy", fmt.Sprintf("event %d", i)))
	}

	ctrl.streamMu.Lock()
	queue := ctrl.watchQueue
	ctrl.streamRunning = false
	ctrl.streamMu.Unlock()

	require.Len(t, queue, watchQueueLimit)
	assert.Equal(t, "event 10", queue[0].Text, "the oldest go first")
	assert.True(t, strings.HasPrefix(queue[len(queue)-1].Text, "event "))
}
