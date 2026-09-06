package agent

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
)

func planEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)
	return engine
}

// TestObservingThePlanLeavesTheRevisionAndTheCountersWhereTheyStood is the
// read-only contract at the seam that owns the most mutable state in the
// harness: a plan carries a revision, a contract epoch, an audit trail and a
// mutation ledger, and asking about it moves none of them.
func TestObservingThePlanLeavesTheRevisionAndTheCountersWhereTheyStood(t *testing.T) {
	engine := planEngine(t)
	_, err := engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore},
		{Content: "change it", Status: session.PlanPending, Type: session.StepEdit},
	})
	require.NoError(t, err)

	before := engine.Plan()
	counters := engine.planTelemetryManager().PlanTelemetry()

	for range 3 {
		engine.PlanObservation()
	}

	assert.Equal(t, before, engine.Plan(),
		"no step is started, approved, patched or settled by looking at the plan")
	assert.Equal(t, counters, engine.planTelemetryManager().PlanTelemetry(),
		"and the counters count the harness's own work, not the questions about it")
}

// planContract is the smallest v2 work contract the authoring path accepts,
// carrying whatever steps a test needs. The contract prose is here because
// the authoring path requires it — and it is the prose no observation may
// carry back out.
func planContract(items ...session.PlanItem) session.PlanV2 {
	return session.PlanV2{
		Goal:            "ship the observation",
		Approach:        "wire it, then look at it",
		SuccessCriteria: []string{"the category answers"},
		Items:           items,
	}
}

func authoredPlan(t *testing.T, engine *Engine, items ...session.PlanItem) {
	t.Helper()
	_, _, _, err := engine.createPlan(t.Context(), planContract(items...))
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
}

// The plan's own generation markers reach the observation unchanged, so two
// snapshots can be told apart by something sharper than their field values.
func TestTheObservationCarriesThePlansRevisionAndContractEpoch(t *testing.T) {
	engine := planEngine(t)
	authoredPlan(t, engine, session.PlanItem{
		ID: "look", Content: "look around", Status: session.PlanInProgress,
		Type: session.StepExplore, Why: "nothing is known yet", DoneWhen: "the seam is found",
	})

	plan := engine.Plan()
	state := engine.PlanObservation()
	assert.Equal(t, plan.Revision, state.Revision)
	assert.Equal(t, plan.ContractEpoch, state.ContractEpoch)
	assert.True(t, state.Present)
	assert.True(t, state.Approved, "the user approved it")
	assert.Equal(t, "v2", state.Schema)
}

// A snapshot the legacy path wrote is reported as legacy rather than as the
// contract schema it was never written under.
func TestALegacySnapshotIsNamedLegacyRatherThanTheCurrentContract(t *testing.T) {
	engine := planEngine(t)
	_, err := engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	assert.Equal(t, "legacy", engine.PlanObservation().Schema)
}

// The three policy layers are three real objects, and the gap between the
// published one and the projected one is the lag a rebind closes. Reading the
// published policy in place of the projected one would report tool schemas the
// model has never been shown.
func TestThePublishedPolicyLeadsTheProjectedOneUntilTheNextRebind(t *testing.T) {
	engine := planEngine(t)

	settled := engine.PlanObservation()
	require.True(t, settled.Policy.Loaded.Known)
	assert.Equal(t, settled.Policy.Loaded.Types, settled.Policy.Projected.Types,
		"a fresh engine has already projected what it published")
	assert.Equal(t, settled.Policy.Default.Types, settled.Policy.Loaded.Types,
		"and it published what this harness ships")

	require.NoError(t, engine.planRuntime.Apply(plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: session.StepExplore, Tools: []string{"read", "grep"}},
		{Name: session.StepEdit, Tools: []string{"write"}},
	}}))

	stale := engine.PlanObservation()
	assert.Equal(t, []string{"explore", "edit"}, stale.Policy.Loaded.Types,
		"the published policy is the one the next tool call is gated by")
	assert.Equal(t, settled.Policy.Projected.Types, stale.Policy.Projected.Types,
		"the prompt still carries the vocabulary it was rendered with")
	assert.Equal(t, []string{"explore", "edit", "run", "delegate", "integrate"},
		stale.Policy.Default.Types, "and the built-in layer never moves")
	assert.NotEqual(t, settled.Policy.Revision, stale.Policy.Revision,
		"a different published policy is a different fingerprint")

	engine.syncPlanProjection()

	rebound := engine.PlanObservation()
	assert.Equal(t, rebound.Policy.Loaded.Types, rebound.Policy.Projected.Types,
		"the rebind is what closes the gap, and the next observation says so")
}

// Two policies with the same names in the same order are the same policy, and
// the fingerprint is what lets a reader establish that without diffing lists.
func TestThePolicyFingerprintFollowsWhatWasPublished(t *testing.T) {
	engine := planEngine(t)
	original := engine.PlanObservation().Policy.Revision

	renamed := plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: session.StepExplore, Tools: []string{"read"}},
	}}
	require.NoError(t, engine.planRuntime.Apply(renamed))
	assert.NotEqual(t, original, engine.PlanObservation().Policy.Revision)

	require.NoError(t, engine.planRuntime.Apply(plangate.DefaultDefaults()))
	assert.Equal(t, original, engine.PlanObservation().Policy.Revision,
		"publishing the same policy again is the same policy")
}

// The step's tool ceiling is the inherited set its type reaches, which is the
// answer to "what may this step do", not "what tools exist".
func TestTheStepInProgressReportsTheToolsItsTypeReaches(t *testing.T) {
	engine := planEngine(t)
	_, err := engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanCompleted, Type: session.StepExplore},
		{Content: "change it", Status: session.PlanInProgress, Type: session.StepEdit},
		{Content: "run it", Status: session.PlanPending, Type: session.StepRun},
	})
	require.NoError(t, err)

	state := engine.PlanObservation()
	require.True(t, state.Step.Known)
	assert.Equal(t, "edit", state.Step.Type)
	assert.Equal(t, []string{"read", "grep", "find", "ls", "lsp", "write", "edit"},
		state.Step.Tools, "an edit step inherits explore's reach and adds its own")
	assert.NotContains(t, state.Step.Tools, "bash",
		"and reaches nothing a later type introduces")

	assert.Equal(t, 3, state.StepsTotal)
	assert.Equal(t, 2, state.StepsRemaining, "a completed step owes no more work")
	assert.Equal(t, []diag.PlanStatusCount{
		{Status: "pending", Count: 1},
		{Status: "in_progress", Count: 1},
		{Status: "completed", Count: 1},
	}, state.Statuses, "the breakdown keeps a fixed order and omits empty statuses")
}

// Nothing in progress means no step, and the next pending step is not promoted
// into the answer: the gate has not started it and the model has not named it.
func TestWithNothingInProgressNoStepIsPromotedIntoTheAnswer(t *testing.T) {
	engine := planEngine(t)
	_, err := engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanPending, Type: session.StepExplore},
	})
	require.NoError(t, err)

	state := engine.PlanObservation()
	assert.False(t, state.Step.Known)
	assert.Empty(t, state.Step.ID)
	assert.Equal(t, 1, state.StepsRemaining, "the work is still owed, it just has not started")
}

// A just-in-time step's demand and its grant are two facts: the demand is
// authored into the plan, and the grant is the user's, epoch-bound answer.
func TestTheJustInTimeDemandAndTheUsersGrantAreReportedApart(t *testing.T) {
	engine := planEngine(t)
	authoredPlan(t, engine, session.PlanItem{
		ID: "push-tag", Content: "push the tag", Status: session.PlanInProgress,
		Type: session.StepRun, Why: "publishes the build", DoneWhen: "tag is on origin",
		Risk: "a published tag is irreversible", JIT: true,
	})

	wanted := engine.PlanObservation()
	assert.True(t, wanted.Step.JIT, "the step asks for a grant")
	assert.False(t, wanted.Step.JITGranted, "and nobody has given it one")

	_, err := engine.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)

	granted := engine.PlanObservation()
	assert.True(t, granted.Step.JITGranted, "the user's answer reaches the next observation")
}

// A skill is named and accounted for. The catalog is not consulted and no body
// is read, so a name the catalog cannot supply is still reported.
func TestSkillsAreAccountedForByNameWithoutReadingABody(t *testing.T) {
	engine := planEngine(t)
	authoredPlan(t, engine, session.PlanItem{
		ID: "change", Content: "change it", Status: session.PlanInProgress,
		Type: session.StepEdit, Why: "the seam is found", DoneWhen: "it compiles",
		Skills: []string{"no-such-skill"},
	})

	state := engine.PlanObservation()
	require.Len(t, state.Step.Skills, 1)
	assert.Equal(t, "no-such-skill", state.Step.Skills[0].Name)
	assert.Contains(t, []string{diag.PlanSkillQueued, diag.PlanSkillPending},
		state.Step.Skills[0].State, "a name with no body still has a state")

	engine.mu.Lock()
	engine.planSkillsDelivered = map[string]struct{}{"no-such-skill": {}}
	engine.mu.Unlock()

	delivered := engine.PlanObservation()
	require.Len(t, delivered.Step.Skills, 1)
	assert.Equal(t, diag.PlanSkillDelivered, delivered.Step.Skills[0].State,
		"a skill this session already sent is not sent twice, and says so")
}

// A step whose skill the user switched off is still accounted for: the name is
// in the plan either way, and the state is what changed.
func TestASwitchedOffSkillIsReportedAsDisabledRatherThanDropped(t *testing.T) {
	engine := planEngine(t)
	authoredPlan(t, engine, session.PlanItem{
		ID: "change", Content: "change it", Status: session.PlanInProgress,
		Type: session.StepEdit, Why: "the seam is found", DoneWhen: "it compiles",
		Actions: []session.PlanAction{{
			Event:          session.PlanActionOnStepStart,
			Type:           session.PlanActionInjectSkill,
			Skills:         []string{"go-testing"},
			DisabledSkills: []string{"go-testing"},
		}},
	})

	state := engine.PlanObservation()
	require.Len(t, state.Step.Skills, 1)
	assert.Equal(t, diag.PlanSkillDisabled, state.Step.Skills[0].State)
}

// A sub-agent runs one job under its parent's approval and addresses no plan.
// Reporting the built-in policy as a live gate there would describe a contract
// that governs nothing.
func TestASubAgentReportsNoPlanRatherThanAnEmptyOne(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir(), ParentID: "parent"},
	})
	require.NoError(t, err)

	state := engine.PlanObservation()
	require.True(t, state.Known)
	assert.False(t, state.Enabled)
	assert.Empty(t, state.GatePhase)
	assert.False(t, state.Present)
	assert.False(t, state.Telemetry.Known, "no gate means no counters, not counters at zero")
	assert.False(t, state.Policy.Loaded.Known, "and no published policy")
	assert.True(t, state.Policy.Default.Known,
		"what the harness ships is still nameable, and is not what gates this engine")
}

// A nil engine is a wiring gap, and the collector turns it into one
// unavailable category rather than a plausible plan.
func TestANilEngineObservesNothingRatherThanADisabledPlan(t *testing.T) {
	var engine *Engine
	assert.Equal(t, diag.PlanState{}, engine.PlanObservation())
}

// The counter mapping across the diagnostics seam is total on both sides: a
// counter the plan telemetry gains and this mapping does not is a gap, and
// this is the test that would notice it.
func TestEveryPlanCounterCrossesTheSeam(t *testing.T) {
	engine := planEngine(t)
	_, err := engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore,
	}})
	require.NoError(t, err)

	snapshot := engine.planTelemetryManager().PlanTelemetry()
	facts := planTelemetryFrom(snapshot)
	require.True(t, facts.Known)
	assert.Equal(t, snapshot.DraftsAdaptive, facts.DraftsAdaptive)
	assert.Equal(t, snapshot.ApprovalChurn, facts.ApprovalChurn)
	assert.Equal(t, snapshot.ProjectionInjections, facts.ProjectionInjections)
	assert.Equal(t, snapshot.CompletionsSuccess, facts.CompletionsSuccess)

	// The two schemas are fixed and numeric, so equality of their field counts
	// is what makes the mapping above total rather than merely current.
	assert.Equal(t, planTelemetryFieldCount(t), planSnapshotFieldCount(t)+1,
		"the observation adds Known to the counters and nothing else")
}

func planTelemetryFieldCount(t *testing.T) int {
	t.Helper()
	return reflect.TypeFor[diag.PlanTelemetry]().NumField()
}

func planSnapshotFieldCount(t *testing.T) int {
	t.Helper()
	return reflect.TypeFor[plantel.Snapshot]().NumField()
}
