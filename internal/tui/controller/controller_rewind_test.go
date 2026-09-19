package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// seedRewindSession records two finished turns straight into the session
// store and returns the second prompt's entry id.
func seedRewindSession(t *testing.T, ctrl *Controller) string {
	t.Helper()
	require.NoError(t, ctrl.engine.Session().Append(
		llm.Message{Role: llm.RoleUser, Content: "first question"},
		llm.Message{Role: llm.RoleAssistant, Content: "first answer"},
		llm.Message{Role: llm.RoleUser, Content: "second question"},
		llm.Message{Role: llm.RoleAssistant, Content: "second answer"},
	))
	boundaries := ctrl.TurnBoundaries()
	require.Len(t, boundaries, 4)
	return boundaries[2].EntryID
}

// A rewind through the controller moves the cursor, and the replay the
// transcript is rebuilt from shrinks to the branch the cursor now stands on.
func TestControllerRewindShrinksTheReplay(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	anchor := seedRewindSession(t, ctrl)
	require.Len(t, ctrl.ReplaySnapshot().Messages, 4)

	result, err := ctrl.Rewind(anchor)
	require.NoError(t, err)
	assert.Equal(t, "second question", result.Prompt)
	assert.Len(t, ctrl.ReplaySnapshot().Messages, 2)

	_, err = ctrl.UndoRewind()
	require.NoError(t, err)
	assert.Len(t, ctrl.ReplaySnapshot().Messages, 4)
}

// While a reply or a queued prompt is running the cursor stays put: the
// guard is the same one trims and deletions keep.
func TestControllerRefusesToRewindWhileARunIsInFlight(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	anchor := seedRewindSession(t, ctrl)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamMu.Unlock()

	_, err := ctrl.Rewind(anchor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rewind")
	_, err = ctrl.UndoRewind()
	require.Error(t, err)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = false
	ctrl.streamMu.Unlock()

	assert.Len(t, ctrl.ReplaySnapshot().Messages, 4, "a refused rewind moved nothing")
	_, err = ctrl.Rewind(anchor)
	require.NoError(t, err)
}
