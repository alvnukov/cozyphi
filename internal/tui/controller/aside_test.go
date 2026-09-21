package controller

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// drainAside collects the side-question updates the bus carries until one
// of them ends the answer.
func drainAside(t *testing.T, bus *Bus) []session.AsideUpdate {
	t.Helper()
	var updates []session.AsideUpdate
	done := false
	waitForCond(t, 3*time.Second, func() bool {
		for _, m := range bus.Drain() {
			ev, ok := m.(SessionEventMsg)
			if !ok {
				continue
			}
			if update, ok := ev.Event.(session.AsideUpdate); ok {
				updates = append(updates, update)
				done = done || update.State != session.StateStreaming
			}
		}
		return done
	})
	return updates
}

// A side question through the controller streams its answer onto the bus and
// is written to the session file, while the replay a resumed transcript is
// built from stays exactly the conversation it was.
func TestControllerAsideStreamsTheAnswerAndLeavesTheReplayAlone(t *testing.T) {
	srv, requests := textSSEServer(t)
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)
	replayBefore := ctrl.ReplaySnapshot()

	require.NoError(t, ctrl.Aside("what did we do?", ""))
	updates := drainAside(t, bus)

	last := updates[len(updates)-1]
	assert.Equal(t, session.StateComplete, last.State)
	assert.Equal(t, "noted", last.Answer)
	assert.Len(t, requests(), 1)
	waitForCond(t, 2*time.Second, func() bool { return !ctrl.RunActive() })

	assert.Equal(t, replayBefore, ctrl.ReplaySnapshot(), "the replay has no aside row in it")
	for _, msg := range ctrl.ReplaySnapshot().Messages {
		assert.False(t, strings.HasPrefix(msg.Text, "btw:"))
	}
	raw, err := os.ReadFile(ctrl.SessionFile())
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"type":"aside"`)
	assert.Contains(t, string(raw), last.ID, "the file names the aside by the id its row streamed under")
	asides := ctrl.engine.Session().AsideAnchors()
	assert.Len(t, asides, 4, "the aside is no message a new one could be asked about")
}

// While a reply or a queued prompt is running there is no side question, and
// the refusal comes before the model is asked.
func TestControllerRefusesAnAsideWhileARunIsInFlight(t *testing.T) {
	srv, requests := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamMu.Unlock()
	err := ctrl.Aside("side?", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot ask a side question")

	ctrl.streamMu.Lock()
	ctrl.streamRunning = false
	ctrl.promptQueue = []queuedPrompt{{text: "waiting its turn", id: "q1"}}
	ctrl.streamMu.Unlock()
	err = ctrl.Aside("side?", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queued prompt")

	assert.Empty(t, requests(), "no refusal reached the model")
	assert.False(t, ctrl.streamRunning, "a refused question does not hold the pipeline")
}

// A refusal by the session, such as an anchor the feed does not show, also
// comes back before anything runs, and leaves the pipeline free.
func TestControllerAsideAtAnUnknownAnchorIsRefusedAtOnce(t *testing.T) {
	srv, requests := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)

	err := ctrl.Aside("side?", "nothing-like-this")
	var notAnchor *session.NotAsideAnchorError
	require.ErrorAs(t, err, &notAnchor)
	assert.False(t, ctrl.RunActive())
	assert.Empty(t, requests())
}
