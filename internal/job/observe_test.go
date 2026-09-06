package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/job"
)

// observeSpy is a runner that records what it was asked to do and then waits.
// Every method the harness view is forbidden from reaching leaves a mark on
// it, so a test can assert that observing left none.
type observeSpy struct {
	started  chan string
	release  chan struct{}
	runs     chan struct{}
	cancels  chan struct{}
	finished chan struct{}
}

func newObserveSpy() *observeSpy {
	return &observeSpy{
		started:  make(chan string, 8),
		release:  make(chan struct{}),
		runs:     make(chan struct{}, 8),
		cancels:  make(chan struct{}, 8),
		finished: make(chan struct{}, 8),
	}
}

func (s *observeSpy) runner() job.Runner {
	return job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
		s.runs <- struct{}{}
		s.started <- env.Job.ID
		select {
		case <-s.release:
			s.finished <- struct{}{}
			return "done", nil
		case <-ctx.Done():
			s.cancels <- struct{}{}
			return "", ctx.Err()
		}
	})
}

// await blocks until n jobs have reached the runner, so an observation is
// taken of a manager that has actually started them.
func (s *observeSpy) await(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-s.started:
		case <-time.After(5 * time.Second):
			t.Fatal("a job never reached the runner")
		}
	}
}

// A process runs one job manager for every session in it. A session asking
// what it has out must be told about its own assignments and about nothing
// another session created — while still being told how full the process is,
// because that is what refuses its next spawn.
func TestObserveSeparatesThisSessionsAssignmentsFromTheProcessesLoad(t *testing.T) {
	spy := newObserveSpy()
	m := newMgr(t, spy.runner(), job.Options{MaxConcurrent: 4})
	t.Cleanup(func() { close(spy.release) })

	for _, req := range []job.SpawnRequest{
		{Prompt: "a", OwnerID: "alpha", ParentID: "alpha", Role: job.RoleExplore},
		{Prompt: "b", OwnerID: "alpha", ParentID: "alpha", Role: job.RoleWorker},
		{Prompt: "c", OwnerID: "beta", ParentID: "beta", Role: job.RoleReview},
	} {
		_, err := m.Spawn(t.Context(), req)
		require.NoError(t, err)
	}
	spy.await(t, 3)

	state := job.Observe(m, job.OwnerFacts{Enabled: true, OwnerID: "alpha"})

	assert.True(t, state.Known)
	assert.True(t, state.Managed)
	assert.Equal(t, 3, state.LiveProcess, "every session's assignments together — this is what fills the slots")
	assert.Equal(t, 2, state.LiveSession, "and only this session's")
	require.Len(t, state.Assignments, 2)
	roles := []string{state.Assignments[0].Role, state.Assignments[1].Role}
	assert.ElementsMatch(t, []string{"explore", "worker"}, roles,
		"the other session's review assignment is counted in the load and named nowhere")
	for _, assignment := range state.Assignments {
		assert.False(t, job.Status(assignment.Status).Terminal(),
			"a job still in memory has not finished")
	}
}

// The view promised to scan no filesystem, and a finished job lives only
// there. Observing one is therefore impossible by construction rather than by
// a filter — which is the difference between "we do not report it" and "we
// cannot reach it".
func TestObserveReportsOnlyWhatIsInMemoryAndNeverReadsTheStore(t *testing.T) {
	spy := newObserveSpy()
	m := newMgr(t, spy.runner(), job.Options{MaxConcurrent: 4})

	info, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "a", OwnerID: "alpha", ParentID: "alpha"})
	require.NoError(t, err)
	spy.await(t, 1)
	close(spy.release)
	result, err := m.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCompleted, result.Info.Status)

	onDisk, err := m.ListForOwner(t.Context(), "alpha")
	require.NoError(t, err)
	require.Len(t, onDisk, 1, "the store still has the finished job, prompt and result and all")

	state := job.Observe(m, job.OwnerFacts{Enabled: true, OwnerID: "alpha"})

	assert.Empty(t, state.Assignments, "and the harness view reaches none of it")
	assert.Equal(t, 0, state.LiveProcess)
	assert.Equal(t, 0, state.LiveSession)
}

// Reading is reading. Nothing here admits a job, cancels one, waits for one
// or recovers one, and the runner is the witness: it is the only thing a
// spawn would reach.
func TestObservingStartsNothingAndStopsNothing(t *testing.T) {
	spy := newObserveSpy()
	m := newMgr(t, spy.runner(), job.Options{MaxConcurrent: 2})
	t.Cleanup(func() { close(spy.release) })

	_, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "a", OwnerID: "alpha", ParentID: "alpha"})
	require.NoError(t, err)
	spy.await(t, 1)
	require.Len(t, spy.runs, 1)

	for range 5 {
		state := job.Observe(m, job.OwnerFacts{Enabled: true, OwnerID: "alpha"})
		require.Equal(t, 1, state.LiveSession)
	}

	assert.Len(t, spy.runs, 1, "no observation admitted a job")
	assert.Empty(t, spy.cancels, "and none cancelled one")
	assert.Empty(t, spy.finished, "and none waited for one to finish")

	// The slot the live job holds is still held, so the manager is in the
	// state the observations described rather than one they disturbed.
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "b", OwnerID: "alpha", ParentID: "alpha"})
	require.NoError(t, err)
	spy.await(t, 1)
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "c", OwnerID: "alpha", ParentID: "alpha"})
	assert.ErrorIs(t, err, job.ErrBusy)
}

// A process that never built a manager still has ceilings: they decide what
// would happen if one were built, and a reader looking at a refused spawn
// needs them whether or not this process is the one that refused it.
func TestObserveWithoutAManagerStillReportsTheCeilingsThatWouldApply(t *testing.T) {
	state := job.Observe(nil, job.OwnerFacts{Enabled: true, OwnerID: "alpha"})

	assert.True(t, state.Known, "the layer is wired; what is missing is the manager")
	assert.False(t, state.Managed)
	assert.Equal(t, 1, state.MaxDepth)
	assert.Equal(t, 4, state.MaxConcurrent)
	assert.Equal(t, []string{"explore", "worker", "review"}, state.Roles)
	assert.NotNil(t, state.Assignments, "an empty list, not a nil one")
	assert.Empty(t, state.Assignments)
	assert.NotEmpty(t, state.Revision)
}

// The ceilings a manager was actually built with win over the defaults, or
// the view would answer for a manager that does not exist.
func TestObserveReportsTheCeilingsTheManagerWasBuiltWith(t *testing.T) {
	spy := newObserveSpy()
	t.Cleanup(func() { close(spy.release) })
	m := newMgr(t, spy.runner(), job.Options{MaxConcurrent: 3, MaxDepth: 2})

	state := job.Observe(m, job.OwnerFacts{Enabled: true, OwnerID: "alpha"})

	assert.Equal(t, 3, state.MaxConcurrent)
	assert.Equal(t, 2, state.MaxDepth)
}

// What the owner knows about itself — whether sub-agents are on, what role it
// runs under, how deep it sits and what its role pins resolved to — is
// carried through unchanged: this package observes the manager, and the
// owner observes itself.
func TestObserveCarriesTheOwnersOwnFactsThrough(t *testing.T) {
	pins := []diag.RolePin{
		{Role: "explore", Ref: "fast", Model: "haiku", Resolved: true},
		{Role: "worker", Ref: "retired-model"},
	}

	state := job.Observe(nil, job.OwnerFacts{
		Enabled: true,
		OwnerID: "alpha",
		Depth:   1,
		Role:    job.RoleReview,
		Pins:    pins,
	})

	assert.True(t, state.Enabled)
	assert.Equal(t, 1, state.Depth)
	assert.Equal(t, "review", state.Role)
	assert.Equal(t, pins, state.Pins)
}

// The revision is only good for telling two states apart, and that is
// exactly what it has to do: a snapshot taken across a spawn must not look
// like the one taken before it.
func TestTheRevisionChangesWhenAnAssignmentAppears(t *testing.T) {
	spy := newObserveSpy()
	m := newMgr(t, spy.runner(), job.Options{MaxConcurrent: 2})
	t.Cleanup(func() { close(spy.release) })
	owner := job.OwnerFacts{Enabled: true, OwnerID: "alpha"}

	before := job.Observe(m, owner).Revision

	_, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "a", OwnerID: "alpha", ParentID: "alpha"})
	require.NoError(t, err)
	spy.await(t, 1)

	assert.NotEqual(t, before, job.Observe(m, owner).Revision)
}
