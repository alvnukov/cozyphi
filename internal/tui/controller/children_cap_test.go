package controller

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// runningChild is a child that has not finished: its assignment is still live.
func runningChild(jobID string) ChildSession {
	return ChildSession{JobID: jobID, Controller: &Controller{assignment: newAssignment(jobID)}}
}

// finishedChild is a child whose assignment is terminal and whose turn is over.
func finishedChild(jobID string) ChildSession {
	c := &Controller{assignment: newAssignment(jobID)}
	c.assignment.Terminal = true
	return ChildSession{JobID: jobID, Controller: c}
}

func TestAdmitChildCountsChildrenNotUserSessions(t *testing.T) {
	r := &Runtime{sessions: map[*Controller]struct{}{}}
	// Twelve conversations the user opened by hand, no children yet.
	for range childCap {
		r.sessions[&Controller{}] = struct{}{}
	}
	require.NoError(t, r.admitChild(), "the user's tabs must not spend a family slot")
	r.finishChildBuild()
	require.Zero(t, r.childBuilding)
}

func TestAdmitChildRefusesWhenEveryChildIsRunning(t *testing.T) {
	r := &Runtime{sessions: map[*Controller]struct{}{}}
	for i := range childCap {
		r.children = append(r.children, runningChild("job-"+strconv.Itoa(i)))
	}
	err := r.admitChild()
	require.ErrorContains(t, err, "already running")
	require.Len(t, r.children, childCap, "a refusal releases nothing")
}

func TestAdmitChildReleasesOldestFinishedChild(t *testing.T) {
	r := &Runtime{sessions: map[*Controller]struct{}{}}
	for i := range childCap {
		id := "job-" + strconv.Itoa(i)
		if i == 3 || i == 7 {
			r.children = append(r.children, finishedChild(id))
			continue
		}
		r.children = append(r.children, runningChild(id))
	}
	require.NoError(t, r.admitChild())
	r.finishChildBuild()
	ids := make([]string, 0, len(r.children))
	for _, child := range r.children {
		ids = append(ids, child.JobID)
	}
	require.NotContains(t, ids, "job-3", "the oldest finished child gives way first")
	require.Contains(t, ids, "job-7", "only one slot is needed, so the later one stays")
	require.Len(t, r.children, childCap-1)
}

func TestAdmitChildRefusesOnClosedRuntime(t *testing.T) {
	r := &Runtime{sessions: map[*Controller]struct{}{}, closed: true}
	require.ErrorContains(t, r.admitChild(), "runtime is closed")
}
