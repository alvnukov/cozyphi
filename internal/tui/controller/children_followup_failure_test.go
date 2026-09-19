package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestFollowUpBindingFailureTerminalizesReservation(t *testing.T) {
	server, bodies := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	defer ctrl.Close()
	ctrl.assignment = newAssignment("reserved-follow-up")
	failure := errors.New("cannot persist child session binding")
	var runner job.Runner = followUpRunner{controller: ctrl, sessionID: ctrl.engine.SessionID(), prompt: queuedPrompt{text: "follow-up"}}
	marked := false
	_, err := runner.Run(t.Context(), job.RunEnv{
		Job:              job.Meta{ID: "reserved-follow-up"},
		BindSession:      func(string) error { return failure },
		MarkIntervention: func() { marked = true },
	})
	require.ErrorIs(t, err, failure)
	require.True(
		t,
		ctrl.Assignment().Terminal,
		"failed admission must not strand subsequent input in an idle reservation",
	)
	require.True(t, marked)
	require.Empty(t, bodies(), "binding failure must not launch inference")
	visible := false
	for _, msg := range ctrl.bus.Drain() {
		if event, ok := msg.(SessionEventMsg); ok {
			if update, ok := event.Event.(session.AssistantMessageUpdate); ok {
				visible = update.Message.State == session.StateError
			}
		}
	}
	require.True(t, visible, "the retained child must show the binding failure")
	// The Controller remains usable: a fresh admitted assignment can claim it.
	_, err = ctrl.RunAssignment(t.Context(), "retry", "try again")
	require.NoError(t, err)
	require.Len(t, bodies(), 1)
}

// TestFollowUpBindingFailureHandsBackTheQueuedPrompt: a prompt that waited in
// the queue owns no transcript row — the strip is its only trace — so an
// admission that fails before the model saw it must put it back rather than
// leave the user with a refusal and nothing to resubmit.
func TestFollowUpBindingFailureHandsBackTheQueuedPrompt(t *testing.T) {
	ctrl := newInjectController(t, NewBus(nil), textSSEServerURL(t))
	defer ctrl.Close()
	ctrl.assignment = newAssignment("reserved-follow-up")
	queued := queuedPrompt{text: "follow-up", id: "u9", rowOwed: true}
	var runner job.Runner = followUpRunner{controller: ctrl, sessionID: ctrl.engine.SessionID(), prompt: queued}

	_, err := runner.Run(t.Context(), job.RunEnv{
		Job:         job.Meta{ID: "reserved-follow-up"},
		BindSession: func(string) error { return errors.New("cannot persist child session binding") },
	})

	require.Error(t, err)
	require.Len(t, ctrl.promptQueue, 1, "the refused prompt keeps waiting in the strip")
	require.Equal(t, "follow-up", ctrl.promptQueue[0].text)
}

// TestFollowUpBindingFailureDropsAPromptThatAlreadyHasItsRow: an entry that
// owes no row already has one, or never wanted one: an assembled brief, a
// watch wake, a prompt the composer drew. Requeuing it would run it twice.
func TestFollowUpBindingFailureDropsAPromptThatAlreadyHasItsRow(t *testing.T) {
	ctrl := newInjectController(t, NewBus(nil), textSSEServerURL(t))
	defer ctrl.Close()
	ctrl.assignment = newAssignment("reserved-follow-up")
	var runner job.Runner = followUpRunner{
		controller: ctrl, sessionID: ctrl.engine.SessionID(),
		prompt: queuedPrompt{text: "already drawn"},
	}

	_, err := runner.Run(t.Context(), job.RunEnv{
		Job:         job.Meta{ID: "reserved-follow-up"},
		BindSession: func(string) error { return errors.New("cannot persist child session binding") },
	})

	require.Error(t, err)
	require.Empty(t, ctrl.promptQueue)
}

// textSSEServerURL is textSSEServer for tests that never inspect the bodies.
func textSSEServerURL(t *testing.T) string {
	t.Helper()
	server, _ := textSSEServer(t)
	return server.URL
}
