package diag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveAgents is the arrangement worth telling apart: one role pinned to a
// model that resolves, one pinned to a name that no longer does, one left
// alone, and two assignments out under different roles. Every gap between
// the three layers below is one of those.
func liveAgents() diag.AgentsState {
	return diag.AgentsState{
		Known:   true,
		Enabled: true,
		Managed: true,
		Roles:   []string{"explore", "worker", "review"},
		Pins: []diag.RolePin{
			{Role: "explore", Ref: "fast", Model: "haiku", Resolved: true},
			{Role: "worker", Ref: "retired-model"},
			{Role: "review"},
		},
		Depth:         0,
		MaxDepth:      1,
		MaxConcurrent: 4,
		LiveProcess:   3,
		LiveSession:   2,
		Assignments: []diag.AssignmentFacts{
			{Role: "explore", Status: "running"},
			{Role: "worker", Status: "starting"},
		},
		Revision: "p2.r1.l3.s2",
	}
}

func agentFields(t *testing.T, state diag.AgentsState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewAgentCollector(diag.AgentDeps{State: func() diag.AgentsState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryAgents)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// "Nothing is running" is six situations with one symptom, fixed in six
// different places — and two of them are not problems at all.
func TestTheAgentLifecycleSeparatesTheWaysThereCanBeNothingRunning(t *testing.T) {
	tests := []struct {
		name  string
		state func(diag.AgentsState) diag.AgentsState
		want  diag.AgentsLifecycle
	}{
		{
			name:  "switched off in the configuration",
			state: func(s diag.AgentsState) diag.AgentsState { s.Enabled = false; return s },
			want:  diag.AgentsDisabled,
		},
		{
			name:  "no manager to admit a spawn",
			state: func(s diag.AgentsState) diag.AgentsState { s.Managed = false; return s },
			want:  diag.AgentsNoManager,
		},
		{
			name: "a sub-agent that may not nest further",
			state: func(s diag.AgentsState) diag.AgentsState {
				s.Depth, s.Role = 1, "explore"
				return s
			},
			want: diag.AgentsDepthReached,
		},
		{
			name: "every slot in the process taken",
			state: func(s diag.AgentsState) diag.AgentsState {
				s.LiveProcess, s.LiveSession, s.Assignments = 4, 0, nil
				return s
			},
			want: diag.AgentsSaturated,
		},
		{
			name:  "this session has assignments out",
			state: func(s diag.AgentsState) diag.AgentsState { return s },
			want:  diag.AgentsRunning,
		},
		{
			name: "room to spawn and nothing out",
			state: func(s diag.AgentsState) diag.AgentsState {
				s.LiveProcess, s.LiveSession, s.Assignments = 0, 0, nil
				return s
			},
			want: diag.AgentsIdle,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := fieldByKey(t, agentFields(t, tc.state(liveAgents())), diag.KeyAgentsState)
			assert.Equal(t, string(tc.want), state.Effective.Value.Str)
		})
	}
}

// Being switched on and having somewhere to run are two answers. A
// configuration can leave sub-agents on in a process that never built a
// manager, and nothing would be admitted there.
func TestTheConfigurationsAnswerIsNotTheProcessesAnswer(t *testing.T) {
	on := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsState)
	assert.Equal(t, string(diag.AgentsEnabled), on.Configured.Value.Str)
	assert.Equal(t, diag.SourceConfigFile, on.Configured.Source.Kind)
	assert.Equal(t, "agents.enabled", on.Configured.Source.Ref)
	assert.Equal(t, string(diag.AgentsManaged), on.Loaded.Value.Str)

	unmanaged := liveAgents()
	unmanaged.Managed = false
	field := fieldByKey(t, agentFields(t, unmanaged), diag.KeyAgentsState)
	assert.Equal(t, string(diag.AgentsEnabled), field.Configured.Value.Str,
		"the configuration still says yes; nothing built the manager it asked for")
	assert.Equal(t, string(diag.AgentsNoManager), field.Loaded.Value.Str)
}

// A pin that no longer names a model does not fail the spawn — the child
// runs the session's model instead. Seeing the pin next to the inheritance
// is the whole of why "the model I pinned is not the one it ran".
func TestAStalePinIsShownDegradingIntoInheritance(t *testing.T) {
	models := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsModels)

	assert.Equal(t, []string{"explore=fast", "worker=retired-model"}, models.Configured.Value.List,
		"a role the configuration leaves alone contributes nothing to ask for")
	assert.Equal(t, "agents.models", models.Configured.Source.Ref)

	assert.Equal(t, []string{"explore=haiku"}, models.Loaded.Value.List,
		"only the pin that still names a model resolves")

	assert.Equal(t, []string{"explore=haiku", "worker=inherit", "review=inherit"}, models.Effective.Value.List,
		"every role answers, and the stale pin answers with inheritance rather than with the name it kept")
	assert.Contains(t, models.Effective.Source.Ref, "degrades to inheritance rather than")
	assert.Equal(t, diag.ApplyNextTurn, models.Apply, "the next spawn reads the pins again")
}

// The role vocabulary, what this session could still name, and what this
// session is itself are three different answers. The last one explains a
// narrowed tool set that nothing in the configuration accounts for.
func TestARoleIsBothWhatCanBeSpawnedAndWhatThisSessionIs(t *testing.T) {
	root := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsRoles)
	assert.Equal(t, []string{"explore", "worker", "review"}, root.Configured.Value.List)
	assert.Equal(t, diag.SourceBuild, root.Configured.Source.Kind)
	assert.Equal(t, []string{"explore", "worker", "review"}, root.Loaded.Value.List)
	assert.Equal(t, diag.StateUnset, root.Effective.State,
		"a session the process opened runs under no role at all")
	assert.Contains(t, root.Effective.Source.Ref, "opened by the process rather than by a spawn")

	child := liveAgents()
	child.Depth, child.Role = 1, "review"
	field := fieldByKey(t, agentFields(t, child), diag.KeyAgentsRoles)
	assert.Equal(t, "review", field.Effective.Value.Str)
	assert.Empty(t, field.Loaded.Value.List,
		"a sub-agent is already as deep as the ceiling allows, so it could name none of them")
}

// The ceiling is a rule about the process; how deep this session sits is a
// fact about this session. Reporting only the first would leave a refused
// spawn unexplained.
func TestTheNestingCeilingIsReportedAgainstThisSessionsOwnDepth(t *testing.T) {
	root := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsDepth)
	assert.Equal(t, int64(1), root.Configured.Value.Int)
	assert.Equal(t, int64(0), root.Loaded.Value.Int)
	assert.True(t, root.Effective.Value.Bool, "the top of the tree still has room below it")
	assert.Equal(t, diag.ApplyRestart, root.Apply)

	child := liveAgents()
	child.Depth, child.Role = 1, "explore"
	field := fieldByKey(t, agentFields(t, child), diag.KeyAgentsDepth)
	assert.Equal(t, int64(1), field.Loaded.Value.Int)
	assert.False(t, field.Effective.Value.Bool)
	assert.Contains(t, field.Effective.Source.Ref, "carries no spawn tool at all",
		"the ceiling is not the only thing stopping it, and saying so stops a wrong fix")
}

// A spawn is refused because the process is full, not because this session
// is. A session that could only see its own assignments would have no way to
// tell that from a bug — so the aggregate is reported, and nothing else of
// another session's is.
func TestTheProcessWideCountIsReportedWithoutAnotherSessionsContent(t *testing.T) {
	concurrency := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsConcurrency)

	assert.Equal(t, int64(4), concurrency.Configured.Value.Int)
	assert.Equal(t, int64(3), concurrency.Loaded.Value.Int, "every session's assignments together")
	assert.Equal(t, int64(2), concurrency.Effective.Value.Int, "and how many of them are this session's")
	assert.Contains(t, concurrency.Loaded.Source.Ref, "carrying nothing of what another session is doing")
	assert.Equal(t, diag.ScopeProcess, concurrency.Scope)
}

// What this session has out is two tallies of the same assignments, and
// neither carries anything a job was created with.
func TestAssignmentsAreCountedByRoleAndByStatusAndByNothingElse(t *testing.T) {
	assignments := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsAssignments)

	assert.Equal(t, diag.StateNotApplicable, assignments.Configured.State,
		"an assignment exists because a spawn created one, not because a file asked for it")
	assert.Equal(t, []string{"explore=1", "worker=1"}, assignments.Loaded.Value.List)
	assert.Equal(t, []string{"running=1", "starting=1"}, assignments.Effective.Value.List)
	assert.Contains(t, assignments.Effective.Source.Ref, "no prompt, description, summary")
	assert.Equal(t, diag.ApplyImmediate, assignments.Apply)
}

// Developer access is a property of the session the process opened. A parent
// that can ask these questions is not evidence that its children may.
func TestDeveloperAccessIsStatedAsSomethingASpawnDoesNotPassOn(t *testing.T) {
	developer := fieldByKey(t, agentFields(t, liveAgents()), diag.KeyAgentsDeveloper)

	assert.Equal(t, diag.StateNotApplicable, developer.Configured.State)
	assert.Contains(t, developer.Configured.Source.Ref, "granted on the command line at startup")
	assert.True(t, developer.Loaded.Value.Bool, "this session carries the view; that is why it can ask")
	assert.False(t, developer.Effective.Value.Bool)
	assert.Contains(t, developer.Effective.Source.Ref, "not a capability a spawn passes on")
}

// Sub-agents switched off is not the same as a manager with nothing out, and
// a layer nobody wired is neither. Each degrades into its own answer rather
// than into an empty list that would read as "nothing is running".
func TestAnUnansweredAgentLayerDoesNotReadAsAnIdleOne(t *testing.T) {
	perSession := []string{diag.KeyAgentsModels, diag.KeyAgentsAssignments}

	off := liveAgents()
	off.Enabled = false
	for _, key := range perSession {
		field := fieldByKey(t, agentFields(t, off), key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "carries no spawn tool", key)
	}

	unmanaged := liveAgents()
	unmanaged.Managed = false
	for _, key := range perSession {
		field := fieldByKey(t, agentFields(t, unmanaged), key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "no job manager is in force", key)
	}

	for _, field := range agentFields(t, diag.AgentsState{}) {
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, field.Key)
		assert.Equal(t, diag.StateUnavailable, field.Configured.State, field.Key)
	}
}

// The catalog is answered from the declared key set alone, and no key
// addresses one assignment: a key per job would make the catalog depend on
// what is running and would address a job by an id this view never reports.
func TestTheAgentCatalogIsStaticAndAddressesNoSingleAssignment(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewAgentCollector(diag.AgentDeps{State: liveAgents}))

	var keys []string
	for _, entry := range registry.Catalog().Categories {
		if entry.Category == diag.CategoryAgents {
			keys = entry.Keys
		}
	}
	require.NotEmpty(t, keys)
	assert.Equal(t, []string{
		diag.KeyAgentsState,
		diag.KeyAgentsRoles,
		diag.KeyAgentsModels,
		diag.KeyAgentsDepth,
		diag.KeyAgentsConcurrency,
		diag.KeyAgentsAssignments,
		diag.KeyAgentsDeveloper,
	}, keys)

	for _, key := range keys {
		explained, err := registry.Explain(t.Context(), diag.CategoryAgents, key)
		require.NoError(t, err, key)
		assert.Equal(t, key, explained.Field.Key)
	}
	_, err := registry.Explain(t.Context(), diag.CategoryAgents, "agents.job.explore-1")
	assert.Error(t, err, "no key addresses one assignment")
}
