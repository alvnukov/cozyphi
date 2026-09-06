package diag_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// builtinPlanPolicy is the policy this harness ships: five ordered types and
// the tools each introduces.
func builtinPlanPolicy() diag.PlanPolicySet {
	return diag.PlanPolicySet{
		Known:      true,
		Types:      []string{"explore", "edit", "run", "delegate", "integrate"},
		Tools:      []string{"read", "grep", "find", "ls", "lsp", "write", "edit", "bash"},
		Exemptions: []string{"context", "harness", "memory", "plan", "question", "session", "task", "watch"},
	}
}

// activePlan is an owner's answer with every layer populated: a gate in the
// deny phase, a configuration that renamed the type vocabulary, a prompt
// still carrying the previous one, an approved plan with a step running, and
// counters that have moved.
func activePlan() diag.PlanState {
	loaded := diag.PlanPolicySet{
		Known:      true,
		Types:      []string{"explore", "change", "run"},
		Tools:      []string{"read", "grep", "write", "edit", "bash"},
		Exemptions: []string{"context", "harness", "memory", "plan", "question", "session", "task", "watch"},
		Authoring:  "adaptive-minimal",
		TypeModels: []string{"change=some-model"},
		Actions:    []string{"plan_end:compact"},
	}
	projected := loaded
	projected.Types = []string{"explore", "edit", "run"}
	return diag.PlanState{
		Known:     true,
		Enabled:   true,
		Mode:      "useplan",
		GatePhase: "deny",
		Policy: diag.PlanPolicy{
			Default:   builtinPlanPolicy(),
			Loaded:    loaded,
			Projected: projected,
			Revision:  "beef",
		},
		Present:        true,
		Schema:         "v2",
		Approved:       true,
		Revision:       12,
		ContractEpoch:  3,
		StepsTotal:     4,
		StepsRemaining: 2,
		Statuses: []diag.PlanStatusCount{
			{Status: "pending", Count: 1},
			{Status: "in_progress", Count: 1},
			{Status: "completed", Count: 2},
		},
		Step: diag.PlanStep{
			Known:    true,
			ID:       "wire-the-collector",
			Type:     "change",
			Tools:    []string{"read", "grep", "write", "edit"},
			Attempts: 6,
			Actions:  []string{"step_start:inject_skill"},
			Skills: []diag.PlanSkill{
				{Name: "go-testing", State: diag.PlanSkillDelivered},
				{Name: "diag-contract", State: diag.PlanSkillQueued},
				{Name: "release", State: diag.PlanSkillDisabled},
			},
		},
		SkillsPending: 1,
		Telemetry: diag.PlanTelemetry{
			Known:                true,
			Misses:               2,
			StandaloneStarts:     1,
			ApprovalChurn:        3,
			ProjectionInjections: 9,
			ProjectionBytes:      4096,
			CompletionsSuccess:   2,
			DraftsAdaptive:       1,
		},
	}
}

func planFields(t *testing.T, state diag.PlanState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewPlanCollector(diag.PlanDeps{State: func() diag.PlanState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryPlan)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

func TestEveryDeclaredPlanKeyIsAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewPlanCollector(diag.PlanDeps{State: activePlan}))
	fields := planFields(t, activePlan())

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryPlan {
			entry = candidate
		}
	}
	require.NotEmpty(t, entry.Keys, "the catalog lists what can be asked for")

	for _, key := range entry.Keys {
		field := fieldByKey(t, fields, key)
		assert.NotEqual(t, diag.StateUnavailable, field.Effective.State,
			"a key the catalog advertises is answered by an owner that knows: %s", key)
	}
	assert.Len(t, fields, len(entry.Keys), "the answer carries exactly the declared keys")
}

// The six lifecycles are the fixture matrix the ticket asks for, and they are
// one field rather than four booleans because a reader deciding what to do
// next needs the state, not the ingredients.
func TestTheLifecycleTellsTheSixStatesAPlanCanStandIn(t *testing.T) {
	disabled := diag.PlanState{Known: true}
	absent := diag.PlanState{Known: true, Enabled: true}

	draft := activePlan()
	draft.Approved = false
	draft.Step = diag.PlanStep{}

	approved := activePlan()
	approved.Step = diag.PlanStep{}

	closed := activePlan()
	closed.Result = "success"

	for _, tc := range []struct {
		want  string
		state diag.PlanState
	}{
		{diag.PlanLifecycleDisabled, disabled},
		{diag.PlanLifecycleAbsent, absent},
		{diag.PlanLifecycleDraft, draft},
		{diag.PlanLifecycleApproved, approved},
		{diag.PlanLifecycleActive, activePlan()},
		{diag.PlanLifecycleClosed, closed},
	} {
		t.Run(tc.want, func(t *testing.T) {
			lifecycle := fieldByKey(t, planFields(t, tc.state), diag.KeyPlanLifecycle)
			assert.Equal(t, diag.StatePresent, lifecycle.Effective.State)
			assert.Equal(t, tc.want, lifecycle.Effective.Value.Str)
		})
	}
}

// A closed plan is not a plan in progress with the work removed: its contract
// is discharged, and the fields say so rather than reporting an approved plan
// that happens to have nothing running.
func TestAClosedPlanReportsItsResultRatherThanLookingApproved(t *testing.T) {
	open := planFields(t, activePlan())
	result := fieldByKey(t, open, diag.KeyPlanResult)
	assert.Equal(t, diag.StateUnset, result.Effective.State)
	assert.Contains(t, result.Effective.Source.Ref, "still open")

	state := activePlan()
	state.Result = "abandoned"
	closed := planFields(t, state)
	assert.Equal(t, "abandoned", fieldByKey(t, closed, diag.KeyPlanResult).Effective.Value.Str)
	assert.Equal(t, diag.PlanLifecycleClosed,
		fieldByKey(t, closed, diag.KeyPlanLifecycle).Effective.Value.Str)
}

// The sharpest separation this category makes: what the harness ships, what
// configuration compiled, and what the model has actually been told.
func TestThePolicySeparatesTheBuiltInFromTheLoadedAndTheProjected(t *testing.T) {
	fields := planFields(t, activePlan())

	types := fieldByKey(t, fields, diag.KeyPlanPolicyTypes)
	assert.Equal(t, []string{"explore", "edit", "run", "delegate", "integrate"},
		types.Configured.Value.List, "the configured layer is what this harness ships")
	assert.Equal(t, diag.SourceDefault, types.Configured.Source.Kind)

	assert.Equal(t, []string{"explore", "change", "run"}, types.Loaded.Value.List,
		"the loaded layer is what the configuration compiled")
	assert.Equal(t, diag.SourceConfigFile, types.Loaded.Source.Kind)

	assert.Equal(t, []string{"explore", "edit", "run"}, types.Effective.Value.List,
		"the effective layer is the vocabulary the prompt still carries")
	assert.Contains(t, types.Effective.Source.Ref, "lags a published change by one rebind")

	assert.Equal(t, "beef", types.Revision,
		"policy fields carry the published policy's own fingerprint")
}

// A layer that does not exist is not the built-in one: an engine that has
// never rebound has projected nothing, and saying "the defaults" there would
// describe a prompt nobody rendered.
func TestAPolicyLayerThatWasNeverCompiledSaysSoRatherThanBorrowingTheDefault(t *testing.T) {
	state := activePlan()
	state.Policy.Projected = diag.PlanPolicySet{}
	types := fieldByKey(t, planFields(t, state), diag.KeyPlanPolicyTypes)

	assert.Equal(t, diag.StateUnset, types.Effective.State)
	assert.Empty(t, types.Effective.Value.List)
	assert.Contains(t, types.Effective.Source.Ref, "no policy is compiled")
}

// An empty authoring selector means the grammar the harness defaults to. It
// is unset rather than present so nobody reads it as a choice someone made.
func TestAnUnwrittenAuthoringGrammarIsUnsetRatherThanNamed(t *testing.T) {
	authoring := fieldByKey(t, planFields(t, activePlan()), diag.KeyPlanPolicyAuthoring)
	assert.Equal(t, "adaptive-minimal", authoring.Loaded.Value.Str,
		"the configuration named a grammar, and it is the one in force")
	assert.Equal(t, diag.StateUnset, authoring.Configured.State,
		"the built-in policy names none, and that is the answer rather than a guess")
}

// The applied step restriction: the type still permits its tools, and the
// gate admits none of them while nobody has approved the plan.
func TestAnUnapprovedPlanLeavesTheStepsCeilingStandingAndAdmitsNothing(t *testing.T) {
	approved := planFields(t, activePlan())
	tools := fieldByKey(t, approved, diag.KeyPlanStepTools)
	assert.Equal(t, []string{"read", "grep", "write", "edit"}, tools.Loaded.Value.List)
	assert.Equal(t, []string{"read", "grep", "write", "edit"}, tools.Effective.Value.List)

	state := activePlan()
	state.Approved = false
	unapproved := fieldByKey(t, planFields(t, state), diag.KeyPlanStepTools)
	assert.Equal(t, []string{"read", "grep", "write", "edit"}, unapproved.Loaded.Value.List,
		"the step's type permits what it permits whatever the user decided")
	assert.Equal(t, diag.StateUnset, unapproved.Effective.State)
	assert.Contains(t, unapproved.Effective.Source.Ref, "not approved")
}

// A just-in-time step wants a grant it may not have. The demand and the grant
// are two layers because a material change to the contract expires the grant
// and leaves the demand exactly where it was.
func TestAJustInTimeStepSeparatesTheDemandFromTheGrant(t *testing.T) {
	state := activePlan()
	state.Step.JIT = true
	wanted := fieldByKey(t, planFields(t, state), diag.KeyPlanStepJIT)
	assert.True(t, wanted.Loaded.Value.Bool, "the step asks for a grant")
	assert.False(t, wanted.Effective.Value.Bool, "and has not been given one")
	assert.Contains(t, wanted.Effective.Source.Ref, "expires one")

	state.Step.JITGranted = true
	granted := fieldByKey(t, planFields(t, state), diag.KeyPlanStepJIT)
	assert.True(t, granted.Effective.Value.Bool)
	assert.Contains(t, granted.Effective.Source.Ref, "current contract epoch")

	ordinary := fieldByKey(t, planFields(t, activePlan()), diag.KeyPlanStepJIT)
	assert.Equal(t, diag.StatePresent, ordinary.Effective.State)
	assert.False(t, ordinary.Effective.Value.Bool)
	assert.Contains(t, ordinary.Effective.Source.Ref, "approving the plan approved it")
}

// Skills are named and accounted for, and that is all: four states, no body.
func TestSkillsAreReportedByNameAndApplicationState(t *testing.T) {
	fields := planFields(t, activePlan())

	skills := fieldByKey(t, fields, diag.KeyPlanStepSkills)
	assert.Equal(t, []string{
		"go-testing=" + diag.PlanSkillDelivered,
		"diag-contract=" + diag.PlanSkillQueued,
		"release=" + diag.PlanSkillDisabled,
	}, skills.Effective.Value.List)
	assert.Contains(t, skills.Effective.Source.Ref, "no skill body is read or carried")

	assert.Equal(t, int64(1),
		fieldByKey(t, fields, diag.KeyPlanSkillsPending).Effective.Value.Int)
}

// Without a step in progress the step fields are unset for one stated reason,
// not quietly filled from the next pending step the gate has not started.
func TestWithNoStepInProgressTheStepFieldsSayWhyRatherThanGuessing(t *testing.T) {
	state := activePlan()
	state.Step = diag.PlanStep{}
	fields := planFields(t, state)

	for _, key := range []string{
		diag.KeyPlanStepCurrent,
		diag.KeyPlanStepType,
		diag.KeyPlanStepTools,
		diag.KeyPlanStepJIT,
		diag.KeyPlanStepAttempts,
		diag.KeyPlanStepActions,
		diag.KeyPlanStepSkills,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnset, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "no step is in progress", key)
	}
}

// A session with no plan is not a session with an empty plan: the counters
// are zero either way, and the provenance is what tells the two apart.
func TestASessionWithNoPlanIsToldApartFromOneWithAnEmptyPlan(t *testing.T) {
	fields := planFields(t, diag.PlanState{Known: true, Enabled: true})

	schema := fieldByKey(t, fields, diag.KeyPlanSchema)
	assert.Equal(t, diag.StateUnset, schema.Effective.State)
	assert.Contains(t, schema.Effective.Source.Ref, "no durable plan has been made")

	approved := fieldByKey(t, fields, diag.KeyPlanApproved)
	assert.Equal(t, diag.StateUnset, approved.Effective.State,
		"nobody declined to approve a plan that does not exist")

	total := fieldByKey(t, fields, diag.KeyPlanStepsTotal)
	assert.Equal(t, int64(0), total.Effective.Value.Int)
	assert.Contains(t, total.Effective.Source.Ref, "no durable plan has been made")
}

// A sub-agent has no plan gate. Saying "disabled" once is the honest answer;
// reporting the built-in policy as if it were gating something is not.
func TestASessionWithoutAPlanGateReportsNoPlanAndNoCounters(t *testing.T) {
	fields := planFields(t, diag.PlanState{Known: true, Mode: "useplan"})

	enabled := fieldByKey(t, fields, diag.KeyPlanEnabled)
	assert.False(t, enabled.Effective.Value.Bool)
	assert.Contains(t, enabled.Effective.Source.Ref, "carries one job rather than a plan")

	assert.Equal(t, diag.StateUnset,
		fieldByKey(t, fields, diag.KeyPlanGatePhase).Effective.State)

	telemetry := fieldByKey(t, fields, diag.KeyPlanTelemetryGate)
	assert.Equal(t, diag.StateUnset, telemetry.Effective.State)
	assert.Contains(t, telemetry.Effective.Source.Ref, "not the same as counters at zero")
}

// The twenty-four counters reach the reader as numbers under fixed names, and
// the schema behind them has no member a plan's prose could arrive in.
func TestTelemetryIsCountersUnderFixedNamesAndCarriesNoRevision(t *testing.T) {
	fields := planFields(t, activePlan())

	gate := fieldByKey(t, fields, diag.KeyPlanTelemetryGate)
	assert.Equal(t, []string{
		"plan_misses=2",
		"transition_conflicts=0",
		"idempotent_retries=0",
		"standalone_starts=1",
		"plan_only_rounds=0",
	}, gate.Effective.Value.List)
	assert.Empty(t, gate.Revision,
		"session totals describe no generation, so they claim no revision")

	projection := fieldByKey(t, fields, diag.KeyPlanTelemetryProjection)
	assert.Equal(t, []string{"injections=9", "bytes_total=4096", "bytes_last=0"},
		projection.Effective.Value.List)
}

// The plan's own generation marker: a revision alone cannot say whether the
// contract moved, so both counters ride every plan field.
func TestPlanFieldsCarryTheSnapshotRevisionAndTheContractEpoch(t *testing.T) {
	fields := planFields(t, activePlan())
	assert.Equal(t, "r12.e3", fieldByKey(t, fields, diag.KeyPlanLifecycle).Revision)
	assert.Equal(t, int64(12), fieldByKey(t, fields, diag.KeyPlanRevision).Effective.Value.Int)
	assert.Equal(t, int64(3), fieldByKey(t, fields, diag.KeyPlanContractEpoch).Effective.Value.Int)
}

// The leak contract, stated as a test: a plan is where the model's own prose
// lives durably, and none of it — nor a mutation id — may leave through here.
func TestThePlanViewCarriesNoPlanTextEvidenceOrMutationToken(t *testing.T) {
	state := activePlan()
	state.Step.ID = "wire-the-collector"
	fields := planFields(t, state)

	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	rendered := string(encoded)

	for _, forbidden := range []string{
		"SENTINEL-GOAL",
		"SENTINEL-APPROACH",
		"SENTINEL-EVIDENCE",
		"SENTINEL-MUTATION",
		"workingContext",
		"doneWhen",
		"evidenceRefs",
		"mutation",
	} {
		assert.NotContains(t, rendered, forbidden,
			"a plan's prose and its replay tokens have no member to arrive in")
	}
	assert.Contains(t, rendered, "wire-the-collector",
		"the step id is the identity a plan_step names, and it is sayable")
	assert.Contains(t, rendered, "go-testing=delivered",
		"a skill is accounted for by name and state")
}

// The snapshot must not alias the owner's slices: a reader that sorts a list
// it was handed must not reorder the plan behind it.
func TestThePlanSnapshotIsDetachedFromTheOwnersState(t *testing.T) {
	state := activePlan()
	fields := planFields(t, state)

	types := fieldByKey(t, fields, diag.KeyPlanPolicyTypes)
	types.Loaded.Value.List[0] = "mutated"
	skills := fieldByKey(t, fields, diag.KeyPlanStepSkills)
	skills.Effective.Value.List[0] = "mutated"

	assert.Equal(t, "explore", state.Policy.Loaded.Types[0])
	assert.Equal(t, "go-testing", state.Step.Skills[0].Name)
}

// A wiring gap is one unavailable category, never an invented plan.
func TestAPlanCollectorWithNoAccessorReportsUnavailableRatherThanNoPlan(t *testing.T) {
	fields := planFields(t, diag.PlanState{})
	for _, field := range fields {
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, field.Key)
	}
	assert.Empty(t, fieldByKey(t, fields, diag.KeyPlanLifecycle).Revision,
		"an unobserved owner has no generation to name")
}

func TestAnUnknownPlanKeyIsRefusedRatherThanAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewPlanCollector(diag.PlanDeps{State: activePlan}))

	_, err := registry.Explain(t.Context(), diag.CategoryPlan, "plan.goal")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyPlanLifecycle,
		"the refusal names what can be asked for instead")
}
