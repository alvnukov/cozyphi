package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdleCloseSealsAssignmentAdmission(t *testing.T) {
	c := &Controller{bus: NewBus(nil)}
	t.Cleanup(c.Close)
	require.True(t, c.BeginCloseIfIdle())
	_, err := c.RunAssignment(t.Context(), "late-followup", "must not start")
	require.ErrorContains(t, err, "idle open session")
}

func TestIdleClosePreservesAcceptedAssignment(t *testing.T) {
	c := &Controller{bus: NewBus(nil), assignment: newAssignment("accepted-followup")}
	t.Cleanup(c.Close)
	// Admission won the lock, but the runner has not published its first activity.
	// The idle-close path must request confirmation rather than cancel the reservation.
	require.False(t, c.BeginCloseIfIdle())
	snapshot := c.Assignment()
	require.False(t, snapshot.Terminal)
	require.False(t, snapshot.StopRequested)
	require.Equal(t, "accepted-followup", snapshot.JobID)
}
