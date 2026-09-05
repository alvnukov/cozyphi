package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestChildInterruptBeforeViewReadyStopsOnLeave(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"unexpected execution\"}}]}\n\ndata: [DONE]\n\n",
		)
	}))
	defer server.Close()
	parent := newInjectController(t, NewBus(nil), server.URL)
	defer parent.Close()
	parent.runtime.EnableInteractiveChildren()
	parent.Cancel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	info, err := parent.jobs.SpawnWithRunner(ctx, job.SpawnRequest{
		Prompt: "initial assignment", Role: job.RoleExplore,
		OwnerID: parent.jobOwnerID, ParentID: parent.engine.SessionID(),
		WorkDir: parent.cwd, ParentWorkspace: parent.cwd,
	}, parent.bindJobRunner(parent.ModelConfig(), parent.Hooks(), nil))
	require.NoError(t, err)
	waitForCond(t, 5*time.Second, func() bool { return len(parent.runtime.Children()) == 1 })
	child := parent.runtime.Children()[0]
	waitForCond(t, 5*time.Second, func() bool { return child.Controller.Assignment().JobID == info.ID })
	sessionID := child.Controller.SessionID()
	require.ErrorContains(t, child.Controller.Clear(), "assignment")
	child.Controller.Cancel()
	require.Equal(t, TurnInterrupted, child.Controller.Assignment().Turn)
	require.False(t, child.Controller.Assignment().Terminal)
	require.ErrorContains(t, child.Controller.Clear(), "assignment")
	_, resumeErr := child.Controller.Resume(parent.SessionID())
	require.ErrorContains(t, resumeErr, "assignment")
	require.Equal(t, sessionID, child.Controller.SessionID())
	child.Controller.LeaveAssignment()
	require.True(t, child.Controller.Assignment().StopRequested)
	require.True(t, child.Controller.Assignment().Terminal)
	child.Ready(nil)
	result, err := parent.jobs.Wait(ctx, info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCancelled, result.Info.Status)
	require.Zero(t, requests.Load())
}

func TestReservedChildContinuationWaitsForAttachmentAndSurvivesLeave(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"continued\"}}]}\n\ndata: [DONE]\n\n",
		)
	}))
	defer server.Close()
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	defer ctrl.Close()
	// Hold the production attachment handshake, not inference or a scheduler.
	attached := newChildAttachment()
	ctrl.childAttached = attached
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := ctrl.RunAssignment(ctx, "reserved", "original"); finished <- err }()
	waitForCond(t, 5*time.Second, func() bool { return ctrl.Assignment().JobID == "reserved" })
	ctrl.Cancel()
	ctrl.StartPrompt("continue instead", nil, "queued-continuation")
	require.Equal(t, TurnInterrupted, ctrl.Assignment().Turn, "continuation cannot bypass View readiness")
	ctrl.LeaveAssignment()
	require.False(t, ctrl.Assignment().Terminal, "accepted continuation is not abandonment")
	attached.finish(nil)
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.EqualValues(t, 1, requests.Load(), "only the continuation runs, not the interrupted original")
}

func TestStoppedReservationRetainsStopReasonWhenRunnerClaimsIt(t *testing.T) {
	server, bodies := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	defer ctrl.Close()
	// This is the reservation installed by newChild/startFollowUpLocked before
	// the admitted runner can acquire streamMu, not a second execution queue.
	ctrl.assignment = newAssignment("reserved")
	ctrl.Cancel()
	ctrl.LeaveAssignment()
	_, err := ctrl.RunAssignment(t.Context(), "reserved", "must not run")
	require.ErrorIs(t, err, ErrLeftInterrupted)
	require.Empty(t, bodies())
}

func TestFollowUpCanReuseAlreadyObservedAttachment(t *testing.T) {
	server, bodies := textSSEServer(t)
	ctrl := newInjectController(t, NewBus(nil), server.URL)
	defer ctrl.Close()
	attached := newChildAttachment()
	attached.finish(nil)
	<-attached.done // The initial runner observed Ready, but has not cleared its field.
	ctrl.childAttached = attached
	ctrl.assignment = newAssignment("new-follow-up")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := ctrl.RunAssignment(ctx, "new-follow-up", "follow-up")
	require.NoError(t, err, "readiness must remain observable by a later admitted assignment")
	require.Len(t, bodies(), 1)
}
