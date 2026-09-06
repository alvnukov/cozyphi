package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// agentField asks one session what it can say about sub-agents, through the
// same path the harness tool takes.
func agentField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryAgents, key)
	require.NoError(t, err)
	return explained.Field
}

// watchField asks one session what it can say about its watches.
func watchField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryDiagnostics, key)
	require.NoError(t, err)
	return explained.Field
}

// heldJob puts one assignment in flight under the given owner and leaves it
// there until the test ends. It is the only way to make a session's live job
// state real: the runtime's own runner refuses an unbound spawn.
func heldJob(t *testing.T, rt *Runtime, ownerID string, role job.Role) {
	t.Helper()
	started := make(chan struct{})
	_, err := rt.jobs.SpawnWithRunner(t.Context(), job.SpawnRequest{
		Prompt: "look something up", OwnerID: ownerID, ParentID: ownerID, Role: role,
	}, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}))
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the assignment never reached its runner")
	}
}

// One process runs one job manager for every session in it. A session must be
// told how full the process is, because that is what refuses its next spawn —
// and must be told nothing else about what another session is doing.
func TestASessionSeesItsOwnAssignmentsAndOnlyTheProcessesLoad(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	first, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	second, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	heldJob(t, rt, first.jobOwnerID, job.RoleExplore)
	heldJob(t, rt, first.jobOwnerID, job.RoleWorker)
	heldJob(t, rt, second.jobOwnerID, job.RoleReview)

	load := agentField(t, first, diag.KeyAgentsConcurrency)
	assert.Equal(t, int64(3), load.Loaded.Value.Int, "every session's assignments fill the same slots")
	assert.Equal(t, int64(2), load.Effective.Value.Int, "and two of them are this session's")

	assignments := agentField(t, first, diag.KeyAgentsAssignments)
	assert.ElementsMatch(t, []string{"explore=1", "worker=1"}, assignments.Loaded.Value.List)
	assert.NotContains(t, fmt.Sprintf("%+v", assignments), "review",
		"the other session's assignment is counted in the load and named nowhere")

	other := agentField(t, second, diag.KeyAgentsAssignments)
	assert.Equal(t, []string{"review=1"}, other.Loaded.Value.List,
		"and each session gets the same answer about itself")
	assert.Equal(t, int64(1), agentField(t, second, diag.KeyAgentsConcurrency).Effective.Value.Int)
}

// A watch manager belongs to one session and nothing about it is persisted,
// so a session cannot reach another's watches even in principle. That is
// worth pinning: it is the reason this category needs no scoping of its own.
func TestASessionSeesOnlyTheWatchesItStarted(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	first, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	second, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	// A timer runs no command at all, so the arrangement starts no process.
	for _, label := range []string{"stand up", "check the deploy"} {
		_, err := first.watches.Start(watch.Spec{Label: label, Every: watch.MinInterval})
		require.NoError(t, err)
	}
	_, err = second.watches.Start(watch.Spec{Label: "stretch", Every: watch.MinInterval})
	require.NoError(t, err)

	assert.Equal(t, int64(2), watchField(t, first, diag.KeyWatchesCount).Effective.Value.Int)
	assert.Equal(t, int64(1), watchField(t, second, diag.KeyWatchesCount).Effective.Value.Int)
	assert.Equal(t, string(diag.WatchesRunning), watchField(t, first, diag.KeyWatchesState).Effective.Value.Str)
	assert.Equal(t, []string{"timer=2"}, watchField(t, first, diag.KeyWatchesShapes).Effective.Value.List)
	assert.Equal(t, int64(6), watchField(t, first, diag.KeyWatchesLimits).Effective.Value.Int,
		"eight live slots less the two this session is using")
}

// A watch is started with words someone typed, and a session's own label is
// as likely to carry a token as anything else in the process. What leaves is
// a shape and a count, so there is nowhere for the words to land.
func TestNoWatchLabelOrCommandReachesTheAnswer(t *testing.T) {
	const sentinel = "sk-live-TUI-SENTINEL"
	rt, ws := developerRuntime(t, true)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	_, err = c.watches.Start(watch.Spec{Label: "poll " + sentinel, Every: watch.MinInterval})
	require.NoError(t, err)
	require.Contains(t, c.watches.List()[0].Label, sentinel,
		"the manager still has it, which is why it is worth checking that the answer does not")

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryDiagnostics)
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprintf("%+v", snapshot), sentinel)
}

// The capability is the user's, fixed on the process at startup. A sub-agent
// of a developer session is still a sub-agent — and the session that can ask
// says so, rather than leaving a reader to assume the answer either way.
func TestASessionStatesThatItsChildrenDoNotInheritTheCapability(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	rt.EnableInteractiveChildren()
	parent, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	developer := agentField(t, parent, diag.KeyAgentsDeveloper)
	assert.True(t, developer.Loaded.Value.Bool, "this session carries the view; that is why it can ask")
	assert.False(t, developer.Effective.Value.Bool)

	child, err := rt.newChild(parent, job.Meta{
		ID: "job-1", Role: job.RoleExplore, ParentID: "parent-conversation", WorkDir: ws.cwd,
	}, &agent.EngineOpts{
		Model:       parent.ModelConfig(),
		MaxRounds:   4,
		SessionOpts: agent.SessionOpts{Cwd: ws.cwd, SessionDir: parent.SessionDir(), Persist: true},
	})
	require.NoError(t, err)
	t.Cleanup(child.Close)

	assert.Nil(t, child.diagnostics, "and the child has no way to ask at all")
}

// A session that is itself a sub-agent runs under a role and sits as deep as
// the ceiling allows, and both are why it carries no spawn tool. Reading them
// off the session is the difference between a bug and a boundary.
func TestAChildSessionObservesItsOwnRoleAndDepth(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	rt.EnableInteractiveChildren()
	parent, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	child, err := rt.newChild(parent, job.Meta{
		ID: "job-1", Role: job.RoleReview, ParentID: "parent-conversation", WorkDir: ws.cwd,
	}, &agent.EngineOpts{
		Model:       parent.ModelConfig(),
		MaxRounds:   4,
		SessionOpts: agent.SessionOpts{Cwd: ws.cwd, SessionDir: parent.SessionDir(), Persist: true},
	})
	require.NoError(t, err)
	t.Cleanup(child.Close)

	// The child cannot ask, so the question is put to its state directly —
	// the same projection the collector reads.
	state := child.agentsState()

	assert.Equal(t, "review", state.Role)
	assert.Equal(t, 1, state.Depth)
	assert.False(t, state.Managed,
		"a child holds no job manager, so its refusal to spawn is a boundary rather than a full process")

	root := parent.agentsState()
	assert.Empty(t, root.Role, "a session the process opened runs under no role at all")
	assert.Equal(t, 0, root.Depth)
	assert.True(t, root.Managed)
}

// Reading is reading: no assignment is admitted, cancelled or waited for, and
// no watch is started, stopped or read.
func TestObservingAgentsAndWatchesLeavesBothExactlyAsTheyWere(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	heldJob(t, rt, c.jobOwnerID, job.RoleExplore)
	_, err = c.watches.Start(watch.Spec{Label: "stand up", Every: watch.MinInterval})
	require.NoError(t, err)

	jobs, watches := rt.jobs.LiveCountForOwner(c.jobOwnerID), c.watches.List()
	require.Equal(t, 1, jobs)
	require.Len(t, watches, 1)

	for range 3 {
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryAgents)
		require.NoError(t, err)
		_, err = c.diagnostics.Snapshot(t.Context(), diag.CategoryDiagnostics)
		require.NoError(t, err)
	}

	assert.Equal(t, jobs, rt.jobs.LiveCountForOwner(c.jobOwnerID))
	assert.Equal(t, watches, c.watches.List(),
		"same watches, same live flags, same event counts")
	assert.Equal(t, 1, c.watches.Live())
}
