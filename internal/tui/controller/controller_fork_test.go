package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fork through the controller writes a second session beside this one and
// leaves this one where it was: same cursor, same replay, same file.
func TestControllerForkWritesASecondSessionAndMovesNothing(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	anchor := seedRewindSession(t, ctrl)

	before, err := os.ReadFile(ctrl.SessionFile())
	require.NoError(t, err)

	result, err := ctrl.Fork(anchor)
	require.NoError(t, err)
	assert.Equal(t, "second question", result.Prompt,
		"a fork before a prompt hands its text to the tab that opens")
	assert.Equal(t, anchor, result.Anchor)
	assert.NotEqual(t, ctrl.SessionFile(), result.File)
	assert.Equal(t, filepath.Dir(ctrl.SessionFile()), filepath.Dir(result.File))

	copied, err := os.ReadFile(result.File)
	require.NoError(t, err)
	assert.Contains(t, string(copied), "first answer")
	assert.NotContains(t, string(copied), "second question",
		"the copy stops before the prompt it was cut at")

	assert.Len(t, ctrl.ReplaySnapshot().Messages, 4, "the session forked from is untouched")
	after, err := os.ReadFile(ctrl.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// A fork with no anchor copies the conversation as it stands, which is what
// /fork without an id asks for.
func TestControllerForkWithoutAnAnchorCopiesEverything(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)

	result, err := ctrl.Fork("")
	require.NoError(t, err)
	assert.Empty(t, result.Prompt, "a copy that stops after an answer hands nothing over")

	copied, err := os.ReadFile(result.File)
	require.NoError(t, err)
	assert.Contains(t, string(copied), "second answer")
}

// While a reply or a queued prompt is running there is no fork: the branch
// the copy would be taken from is still being written.
func TestControllerRefusesToForkWhileARunIsInFlight(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)
	dir := filepath.Dir(ctrl.SessionFile())
	before, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)

	ctrl.streamMu.Lock()
	ctrl.streamRunning = true
	ctrl.streamMu.Unlock()

	_, err = ctrl.Fork("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot fork")

	ctrl.streamMu.Lock()
	ctrl.streamRunning = false
	ctrl.promptQueue = []queuedPrompt{{text: "waiting its turn", id: "q1"}}
	ctrl.streamMu.Unlock()

	_, err = ctrl.Fork("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queued prompt")

	after, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "a refused fork writes nothing")

	ctrl.streamMu.Lock()
	ctrl.promptQueue = nil
	ctrl.streamMu.Unlock()
	_, err = ctrl.Fork("")
	require.NoError(t, err)
}

// The places a fork may be taken keep the entry the cursor stands on, which
// the rewind list drops: that one is the copy of the whole conversation.
func TestControllerForkBoundariesKeepTheCursorsEntry(t *testing.T) {
	srv, _ := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), srv.URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)

	forks := ctrl.ForkBoundaries()
	require.Len(t, forks, 4)
	assert.Len(t, ctrl.TurnBoundaries(), 3)
	assert.Equal(t, "after second answer", forks[3].Preview)
}
