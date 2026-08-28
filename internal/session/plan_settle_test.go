package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settleFixture returns a manager holding an approved v2 plan: "prev" is
// in_progress (the step a working call settles), "next" is pending (the
// target the same call starts), at revision 2 — the same shape
// transitionFixture pins.
func settleFixture(t *testing.T) *Manager {
	t.Helper()
	contract := v2Fixture()
	contract.Items = []PlanItem{
		{
			ID:       "prev",
			Content:  "settled by the working call",
			Status:   PlanInProgress,
			Type:     StepEdit,
			Why:      "piggyback needs a previous step",
			DoneWhen: "completed in the same round",
		},
		{
			ID:       "next",
			Content:  "started by the same call",
			Status:   PlanPending,
			Type:     StepEdit,
			Why:      "piggyback needs a target",
			DoneWhen: "in_progress after dispatch",
		},
	}
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, m.Plan().Approved)
	return m
}

func settlePayload(mutation string) PlanSettle {
	fresh := "fresh scratchpad"
	return PlanSettle{
		MutationID: mutation,
		Complete: &PlanTransition{
			Action:   TransitionComplete,
			StepID:   "prev",
			Outcome:  "prev concluded",
			Evidence: "focused tests",
		},
		WorkingContext: &fresh,
		StartStepID:    "next",
	}
}

func TestSettlePlanFromCallAppliesEveryPartInOneRevision(t *testing.T) {
	m := settleFixture(t)
	before := m.Plan().Revision

	plan, result, err := m.SettlePlanFromCall(settlePayload("settle-one"))
	require.NoError(t, err)

	assert.True(t, result.Completed)
	assert.True(t, result.ContextUpdated)
	assert.True(t, result.Started)
	assert.False(t, result.Replayed)
	assert.Equal(t, "prev", result.StepID)
	assert.Equal(t, before+1, result.Revision)
	assert.Len(t, result.EventIDs, 2, "complete and start each append one event")

	prev := plan.Items[0]
	assert.Equal(t, PlanCompleted, prev.Status)
	assert.Equal(t, "prev concluded", prev.Outcome)
	assert.Equal(t, "focused tests", prev.Evidence)
	assert.Equal(t, PlanInProgress, plan.Items[1].Status)
	assert.Equal(t, "fresh scratchpad", plan.WorkingContext)
	assert.True(t, plan.Approved, "operational writes keep approval")
	assert.Equal(t, before+1, m.Plan().Revision, "every applied part shares one revision")
}

func TestSettlePlanFromCallReplaysMutationWithoutDuplicates(t *testing.T) {
	m := settleFixture(t)

	_, first, err := m.SettlePlanFromCall(settlePayload("settle-one"))
	require.NoError(t, err)
	before := m.Plan()
	plan, second, err := m.SettlePlanFromCall(settlePayload("settle-one"))
	require.NoError(t, err)

	assert.True(t, second.Replayed)
	assert.Equal(t, first.Revision, second.Revision)
	assert.Equal(t, before.Revision, plan.Revision)
	assert.Equal(t, before.Events, plan.Events, "a replay appends no duplicate events")
}

func TestSettlePlanFromCallCollisionRefused(t *testing.T) {
	m := settleFixture(t)
	_, _, err := m.SettlePlanFromCall(settlePayload("settle-one"))
	require.NoError(t, err)

	// Same mutation id under different work: a collision, not a replay.
	payload := settlePayload("settle-one")
	payload.Complete.StepID = "next"
	_, _, err = m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, "already used")
}

func TestSettlePlanFromCallInvalidCompleteChangesNothing(t *testing.T) {
	m := settleFixture(t)
	before := m.Plan()

	payload := settlePayload("settle-bad")
	payload.Complete.Outcome = "" // complete without an outcome is invalid
	_, _, err := m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, "outcome is required")

	after := m.Plan()
	assert.Equal(t, before.Revision, after.Revision)
	assert.Equal(t, PlanInProgress, after.Items[0].Status)
	assert.Equal(t, before.WorkingContext, after.WorkingContext,
		"an invalid settle mutates no context either")
}

func TestSettlePlanFromCallUnknownStepRefused(t *testing.T) {
	m := settleFixture(t)
	payload := settlePayload("settle-ghost")
	payload.Complete.StepID = "ghost"
	_, _, err := m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, `step "ghost" not found`)
}

func TestSettlePlanFromCallForgedEvidenceRefRefused(t *testing.T) {
	m := settleFixture(t)
	payload := settlePayload("settle-forged")
	payload.Complete.Evidence = ""
	payload.Complete.EvidenceRefs = []string{"call:never-recorded"}
	_, _, err := m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, "not a successful attempt of this step")
}

func TestSettlePlanFromCallOversizedContextRefused(t *testing.T) {
	m := settleFixture(t)
	before := m.Plan().Revision
	huge := strings.Repeat("w", maxPlanWorkingContextRunes+1)
	payload := settlePayload("settle-huge")
	payload.WorkingContext = &huge
	_, _, err := m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, "working context exceeds")
	assert.Equal(t, before, m.Plan().Revision, "nothing applied")
}

func TestSettlePlanFromCallStartRaceAndTerminalStates(t *testing.T) {
	m := settleFixture(t)

	// A target another call already started is success, not a conflict.
	fresh := "second scratchpad"
	payload := settlePayload("settle-race")
	payload.WorkingContext = &fresh
	_, result, err := m.SettlePlanFromCall(payload)
	require.NoError(t, err)
	require.True(t, result.Started)

	again := settlePayload("settle-race-two")
	again.Complete = nil
	again.StartStepID = "next" // already in_progress
	plan, result, err := m.SettlePlanFromCall(again)
	require.NoError(t, err)
	assert.False(t, result.Started, "a lost start race applies no duplicate event")
	assert.Equal(t, PlanInProgress, plan.Items[1].Status)

	terminal := settlePayload("settle-terminal")
	terminal.Complete = nil
	terminal.StartStepID = "prev" // completed by the first settle
	_, _, err = m.SettlePlanFromCall(terminal)
	require.ErrorContains(t, err, `cannot start step "prev"`)
}

func TestSettlePlanFromCallEmptyAndForeignRefused(t *testing.T) {
	m := settleFixture(t)
	_, _, err := m.SettlePlanFromCall(PlanSettle{MutationID: "settle-empty"})
	require.ErrorContains(t, err, "carries no work")

	payload := settlePayload("settle-block")
	payload.Complete.Blocker = "b"
	payload.Complete.ResumeWhen = "r"
	_, _, err = m.SettlePlanFromCall(payload)
	require.ErrorContains(t, err, "takes no")
}

func TestSettlePlanFromCallRequiresV2Plan(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.SettlePlanFromCall(settlePayload("settle-legacy"))
	require.ErrorContains(t, err, "requires a v2 plan")
}

// TestSettleCompleteFinishesPlan covers the piggyback road of the
// auto-finish: a settle that completes the last active work closes the plan
// in the same write, and the retried mutation replays the close with it.
func TestSettleCompleteFinishesPlan(t *testing.T) {
	m := finishFixture(t,
		PlanItem{
			ID: "prev", Content: "last active work", Status: PlanInProgress,
			Type: StepEdit, Why: "settle road", DoneWhen: "closed in the same write",
		},
		PlanItem{
			ID: "done", Content: "landed earlier", Status: PlanCompleted,
			Type: StepEdit, Why: "neighbor", DoneWhen: "recorded",
		},
	)
	payload := PlanSettle{
		MutationID: "settle-close-1",
		Complete: &PlanTransition{
			Action: TransitionComplete, StepID: "prev",
			Outcome: "prev concluded", Evidence: "focused tests",
			PlanResult: PlanResultSuccess,
		},
	}

	plan, result, err := m.SettlePlanFromCall(payload)
	require.NoError(t, err)
	assert.Equal(t, PlanResultSuccess, result.Closed)
	assert.Equal(t, PlanResultSuccess, plan.Result)
	require.NotNil(t, plan.ClosedAt)
	assert.Equal(t, []string{TransitionComplete, TransitionFinish}, []string{
		plan.Events[len(plan.Events)-2].Action, plan.Events[len(plan.Events)-1].Action,
	})

	_, replay, err := m.SettlePlanFromCall(payload)
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, PlanResultSuccess, replay.Closed)
	assert.Equal(t, plan.Revision, m.Plan().Revision, "a replayed settle adds no revision")
}

// TestSettleFinishRefusesNonTerminalNeighbour: closing names the whole plan,
// so a pending target step refuses the close — the settle changes nothing.
func TestSettleFinishRefusesNonTerminalNeighbour(t *testing.T) {
	m := settleFixture(t)
	payload := settlePayload("settle-close-2")
	payload.Complete.PlanResult = PlanResultSuccess
	payload.StartStepID = "" // starting new work and closing the plan contradict

	_, _, err := m.SettlePlanFromCall(payload)
	require.ErrorContains(
		t, err, "plan_result refuses: 1 step(s) not terminal: next (pending)",
	)
	assert.Empty(t, m.Plan().Result)
	assert.Equal(t, PlanInProgress, m.Plan().Items[0].Status, "the refused settle changed nothing")
}
