// Package planscen is the integration gate for adaptive plan authoring: a
// deterministic scenario suite that walks each scenario through the real
// plangate policy and the real session lifecycle — no permission-gate mocks,
// no stubbed approvals. Every scenario is a falsifiable claim about the whole
// increment (gate + approval + lifecycle) rather than about one unit.
package planscen

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scenarioSession returns a manager holding one approved v2 plan built from
// the given items, at revision 2 — the same real approval path the harness
// drives: strict contract validation, durable replace, then user approval.
func scenarioSession(t *testing.T, items ...session.PlanItem) *session.Manager {
	t.Helper()
	contract := session.PlanV2{
		Goal:            "prove the adaptive authoring increment end to end",
		Approach:        "one deterministic scenario per claim, real gate and lifecycle",
		SuccessCriteria: []string{"every scenario closes or denies as claimed"},
		Items:           items,
	}
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, m.Plan().Approved, "the fixture must reach the approved state")
	return m
}

// v2Step builds one valid v2 step.
func v2Step(id string, typ session.StepType, status session.PlanStatus) session.PlanItem {
	return session.PlanItem{
		ID:       id,
		Content:  "step " + id,
		Status:   status,
		Type:     typ,
		Why:      "scenario " + id,
		DoneWhen: "claimed outcome observed",
	}
}

// complete drives the real transition path for one step; the last complete
// of the last active step may also close the plan with a result.
func complete(
	t *testing.T, m *session.Manager, id, mutation, evidence string, result session.PlanResult,
) session.PlanTransitionResult {
	t.Helper()
	tr := session.PlanTransition{
		Action:     session.TransitionComplete,
		StepID:     id,
		MutationID: mutation,
		Outcome:    "step concluded",
		Evidence:   evidence,
		PlanResult: result,
	}
	plan, res, err := m.TransitionPlan(tr, false)
	require.NoError(t, err)
	require.Equal(t, session.PlanCompleted, stepByID(t, plan, id).Status, "complete must land the step")
	return res
}

func stepByID(t *testing.T, plan session.Plan, id string) session.PlanItem {
	t.Helper()
	for _, item := range plan.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("step %q not found in plan", id)
	return session.PlanItem{}
}

// gate is one real policy check: the compiled policy, the phase, the current
// durable plan, and the call under review.
func gate(p *plangate.Policy, plan session.Plan, tool, stepID string) plangate.Verdict {
	return p.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: tool, Step: plangate.StepRef{ID: stepID}})
}

func defaultPolicy(t *testing.T) *plangate.Policy {
	t.Helper()
	p, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)
	return p
}

// TestScenarioTrivialTaskClosesCleanly: a one-step approved plan walks the
// whole happy path — the gate starts the pending step, one complete with
// evidence closes both the step and the plan as success, and the finished
// plan discharges the gate for whatever comes after.
func TestScenarioTrivialTaskClosesCleanly(t *testing.T) {
	policy := defaultPolicy(t)
	m := scenarioSession(t, v2Step("wire-render", session.StepEdit, session.PlanPending))

	verdict := gate(policy, m.Plan(), "write", "wire-render")
	assert.False(t, verdict.Deny)
	assert.False(t, verdict.Miss)
	assert.Equal(t, "wire-render", verdict.StepID)
	assert.True(t, verdict.StartPending, "the pending step starts before dispatch")

	res := complete(t, m, "wire-render", "close-1", "focused tests", session.PlanResultSuccess)
	assert.Equal(t, session.PlanResultSuccess, res.PlanClosed, "the last complete closes the plan")
	plan := m.Plan()
	assert.Equal(t, session.PlanResultSuccess, plan.Result)
	require.NotNil(t, plan.ClosedAt)

	after := gate(policy, plan, "edit", "ghost-after-close")
	assert.False(t, after.Miss, "a finished plan no longer gates tool calls")
	assert.False(t, after.Deny)
}

// TestScenarioUncertainBugRoutesReadOnly: a diagnosis-shaped step carries
// only read capability — the probe tools pass, the mutating tools deny.
func TestScenarioUncertainBugRoutesReadOnly(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: "explore_bugs", Tools: []string{"read", "grep", "lsp"}},
	}})
	require.NoError(t, err)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		ID: "diagnose-flake", Content: "find the flaky test", Status: session.PlanInProgress, Type: "explore_bugs",
	}}}

	probe := gate(policy, plan, "lsp", "diagnose-flake")
	assert.False(t, probe.Deny)
	assert.Equal(t, "diagnose-flake", probe.StepID)

	mutate := gate(policy, plan, "edit", "diagnose-flake")
	assert.True(t, mutate.Deny, "a read-only step never lends its write capability")
}

// TestScenarioCompoundWorkWalksEveryStep: a three-step plan walks gate,
// transition and completion in order, and only the final complete closes the
// plan — every verdict binds to exactly the step it names.
func TestScenarioCompoundWorkWalksEveryStep(t *testing.T) {
	policy := defaultPolicy(t)
	m := scenarioSession(t,
		v2Step("survey-code", session.StepExplore, session.PlanPending),
		v2Step("apply-edit", session.StepEdit, session.PlanPending),
		v2Step("wire-mcp", session.StepIntegrate, session.PlanPending),
	)

	first := gate(policy, m.Plan(), "read", "survey-code")
	require.True(t, first.StartPending)
	assert.False(t, first.Deny)

	res1 := complete(t, m, "survey-code", "m-1", "grep evidence", "")
	assert.Empty(t, res1.PlanClosed, "work remains, the plan stays open")

	second := gate(policy, m.Plan(), "edit", "apply-edit")
	require.True(t, second.StartPending)
	assert.False(t, second.Deny)
	complete(t, m, "apply-edit", "m-2", "edit landed", "")

	third := gate(policy, m.Plan(), "mcp_inspect", "wire-mcp")
	assert.True(t, third.StartPending)
	assert.False(t, third.Deny)

	final := complete(t, m, "wire-mcp", "m-3", "server tools listed", session.PlanResultSuccess)
	assert.Equal(t, session.PlanResultSuccess, final.PlanClosed)
	assert.Equal(t, session.PlanResultSuccess, m.Plan().Result)
	for _, id := range []string{"survey-code", "apply-edit", "wire-mcp"} {
		assert.Equal(t, session.PlanCompleted, stepByID(t, m.Plan(), id).Status)
	}
}

// TestScenarioReadOnlyRunDeniesEscalation: an explore step may read but not
// write or shell out — both the write and the run capability deny while the
// read capability passes.
func TestScenarioReadOnlyRunDeniesEscalation(t *testing.T) {
	policy := defaultPolicy(t)
	m := scenarioSession(t, v2Step("inspect-state", session.StepExplore, session.PlanInProgress))

	read := gate(policy, m.Plan(), "read", "inspect-state")
	assert.False(t, read.Deny)
	for _, tool := range []string{"write", "bash"} {
		escalated := gate(policy, m.Plan(), tool, "inspect-state")
		assert.True(t, escalated.Deny, "%s must deny on a read-only step", tool)
		assert.NotEmpty(t, escalated.Reason)
	}
}

// TestScenarioNovelNoMatchMissesHonestly: a call naming a step that does not
// exist is a miss with a reason — denied in the deny phase, recorded but not
// blocking in the hint phase — and never resolves to a step id.
func TestScenarioNovelNoMatchMissesHonestly(t *testing.T) {
	policy := defaultPolicy(t)
	m := scenarioSession(t, v2Step("known-work", session.StepEdit, session.PlanInProgress))
	plan := m.Plan()
	call := plangate.ToolCall{Name: "edit", Step: plangate.StepRef{ID: "ghost-step"}}

	denied := policy.Check(plangate.PhaseDeny, plan, call)
	assert.True(t, denied.Miss)
	assert.True(t, denied.Deny)
	assert.Empty(t, denied.StepID, "a ghost step never resolves")
	assert.NotEmpty(t, denied.Reason)

	hinted := policy.Check(plangate.PhaseHint, plan, call)
	assert.True(t, hinted.Miss, "the hint phase still records the miss")
	assert.False(t, hinted.Deny, "the hint phase teaches instead of blocking")
}

// TestScenarioRiskyJITStepAsksThenGrants: a just-in-time step raises its
// approval demand through the verdict — never as a miss — and the real
// approval grant clears it; a later material change kills the grant and the
// demand returns, so approval semantics hold across the whole path.
func TestScenarioRiskyJITStepAsksThenGrants(t *testing.T) {
	policy := defaultPolicy(t)
	risky := v2Step("push-tag", session.StepRun, session.PlanInProgress)
	risky.JIT = true
	risky.Risk = "a published tag is irreversible"
	m := scenarioSession(t, risky)

	asking := gate(policy, m.Plan(), "bash", "push-tag")
	require.NotNil(t, asking.JIT, "an ungranted JIT step must ask")
	assert.False(t, asking.Miss, "a demand is a handoff, not a miss")
	assert.Contains(t, asking.JIT.Question(), "push-tag")

	_, err := m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)
	granted := gate(policy, m.Plan(), "bash", "push-tag")
	assert.Nil(t, granted.JIT, "the grant clears the demand")
	assert.False(t, granted.Deny)

	actionBump := session.PlanPatchOp{
		Op:      session.PlanPatchUpdateStep,
		ID:      "push-tag",
		Content: session.PatchValue[string]{Set: true, Value: "push the release tag now"},
	}
	_, _, err = m.PatchPlan(m.Plan().Revision, []session.PlanPatchOp{actionBump}, false)
	require.NoError(t, err)
	require.False(t, m.Plan().Approved, "the material change also resets plan approval")
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err, "the user re-approves the contract, not the step")
	askingAgain := gate(policy, m.Plan(), "bash", "push-tag")
	assert.NotNil(t, askingAgain.JIT,
		"re-approving the plan revives no step grant: the demand returns")
}

// TestScenarioCustomTypeNamesCompileAndGate: a policy configured with custom
// type names gates exactly the tools each name permits, and the session
// persists the custom type so policy validation can see it later.
func TestScenarioCustomTypeNamesCompileAndGate(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: "survey", Tools: []string{"grep", "read"}},
		{Name: "ship", Tools: []string{"bash"}},
	}})
	require.NoError(t, err)
	assert.Contains(t, policy.StepTypes(), "survey")
	assert.Contains(t, policy.StepTypes(), "ship")

	m := session.NewManager(t.TempDir())
	persisted, err := m.ReplacePlan([]session.PlanItem{{
		ID: "map-room", Content: "map the room", Status: session.PlanInProgress, Type: "survey",
	}})
	require.NoError(t, err)
	require.Equal(t, session.StepType("survey"), persisted.Items[0].Type)

	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		ID: "map-room", Content: "map the room", Status: session.PlanInProgress, Type: "survey",
	}}}
	probe := gate(policy, plan, "grep", "map-room")
	assert.False(t, probe.Deny)
	assert.Equal(t, "map-room", probe.StepID)
	assert.True(t, gate(policy, plan, "bash", "map-room").Deny, "survey never borrows ship's tools")
}

// TestScenarioStaleHintTeachesThenApprovalDischarges: while the plan waits
// for approval the hint phase lets a gateable call through with guidance; the
// deny phase blocks the same call; approval discharges the ask; and a patch
// against a stale revision is refused outright.
func TestScenarioStaleHintTeachesThenApprovalDischarges(t *testing.T) {
	policy := defaultPolicy(t)
	contract := session.PlanV2{
		Goal:            "prove the approval boundary",
		Approach:        "hint, deny, approve, then stale revision",
		SuccessCriteria: []string{"each phase behaves as claimed"},
		Items:           []session.PlanItem{v2Step("wait-for-user", session.StepEdit, session.PlanPending)},
	}
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	require.False(t, m.Plan().Approved)
	call := plangate.ToolCall{Name: "edit", Step: plangate.StepRef{ID: "wait-for-user"}}

	hinted := policy.Check(plangate.PhaseHint, m.Plan(), call)
	assert.False(t, hinted.Deny, "the hint phase teaches instead of blocking")
	assert.Empty(t, hinted.Miss)
	assert.NotEqual(t, policy.PromptBlock(plangate.PhaseHint), policy.PromptBlock(plangate.PhaseDeny),
		"the two phases say different things to the model")
	assert.True(t, policy.Check(plangate.PhaseDeny, m.Plan(), call).Deny, "the deny phase blocks the same call")

	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	approved := policy.Check(plangate.PhaseDeny, m.Plan(), call)
	assert.False(t, approved.Deny)
	assert.True(t, approved.StartPending)

	stale := session.PlanPatchOp{
		Op:   session.PlanPatchUpdateStep,
		ID:   "wait-for-user",
		Note: session.PatchValue[string]{Set: true, Value: "stale write"},
	}
	_, _, err = m.PatchPlan(m.Plan().Revision+1, []session.PlanPatchOp{stale}, false)
	require.Error(t, err, "a patch against a stale revision must be refused")
}

// TestScenarioUnavailableToolNeverSurfaces: a tool no configured step type
// permits denies with a reason even when the call names a live step, and the
// policy's visible-tool set keeps it hidden from the model.
func TestScenarioUnavailableToolNeverSurfaces(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: "edit", Tools: []string{"write", "edit"}},
	}})
	require.NoError(t, err)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		v2Step("local-edit", session.StepEdit, session.PlanInProgress),
	}}

	denied := gate(policy, plan, "mcp_call", "local-edit")
	assert.True(t, denied.Deny, "a tool no type permits always denies")
	assert.NotEmpty(t, denied.Reason)

	visible := policy.VisibleTools(plan)
	assert.NotContains(t, visible, "mcp_call")
	assert.Contains(t, visible, "plan", "the plan tool stays available for authoring")
}

// TestScenarioMidplanMaterialAdaptationClosesAsSuccess: the convergence
// scenario. An approved plan mid-flight supersedes an in-progress edit step
// with a run-shaped replacement through the real patch path, the material
// change resets approval, the user re-approves, and the plan still closes as
// success with the superseded step retired — supersede, not cancel.
func TestScenarioMidplanMaterialAdaptationClosesAsSuccess(t *testing.T) {
	inFlight := v2Step("alpha", session.StepEdit, session.PlanInProgress)
	inFlight.Outcome = "edit concluded"
	inFlight.Evidence = "focused tests"
	m := scenarioSession(t, inFlight, v2Step("beta", session.StepRun, session.PlanPending))

	replacement := v2Step("alpha", session.StepRun, session.PlanPending)
	replacement.Why = inFlight.Why
	replacement.DoneWhen = inFlight.DoneWhen
	op := session.PlanPatchOp{
		Op:   session.PlanPatchSupersedeStep,
		ID:   "alpha",
		Step: &replacement,
	}
	// The replacement keeps the retired step's id only as a link target;
	// supersede gives it a fresh id.
	op.Step.ID = "alpha-run"

	plan, summary, err := m.PatchPlan(m.Plan().Revision, []session.PlanPatchOp{op}, false)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha->alpha-run"}, summary.StepsSuperseded)
	retired := stepByID(t, plan, "alpha")
	assert.Equal(t, session.PlanSuperseded, retired.Status)
	assert.Equal(t, "alpha-run", retired.SupersededBy)
	assert.Equal(t, "edit concluded", retired.Outcome, "supersede keeps the recorded outcome")

	assert.False(t, plan.Approved, "a material supersede resets approval: the contract changed")

	_, err = m.SetPlanApproved(true)
	require.NoError(t, err, "the user re-approves the adapted contract")
	fresh := gate(defaultPolicy(t), m.Plan(), "bash", "alpha-run")
	assert.False(t, fresh.Deny, "the replacement gates as its new type")
	assert.True(t, fresh.StartPending)

	complete(t, m, "alpha-run", "adapt-1", "run concluded", "")
	final := complete(t, m, "beta", "adapt-2", "run verified", session.PlanResultSuccess)
	assert.Equal(t, session.PlanResultSuccess, final.PlanClosed, "the adapted plan closes as success")
	closed := m.Plan()
	assert.Equal(t, session.PlanResultSuccess, closed.Result)
	require.NotNil(t, closed.ClosedAt)
	assert.Equal(t, session.PlanSuperseded, stepByID(t, closed, "alpha").Status, "superseded stays retired")
	assert.Equal(t, session.PlanCompleted, stepByID(t, closed, "alpha-run").Status)
	assert.Equal(t, session.PlanCompleted, stepByID(t, closed, "beta").Status)
}
