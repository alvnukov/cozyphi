package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// telemetryFixture returns an approved v2 plan manager: revision 2, step
// "alpha" in the requested status, "beta" completed. Creation and the first
// approval are not churn, not revisions — the table below counts what happens
// after the plan is live.
func telemetryFixture(t *testing.T, status PlanStatus) *Manager {
	t.Helper()
	m := transitionFixture(t, status)
	require.EqualValues(t, 0, m.PlanTelemetry().ApprovalChurn, "the first approval is not churn")
	return m
}

// TestPlanTelemetryCountsOperations is the operation-to-counter contract:
// every counter moves exactly on its public plan operation, and nothing else
// moves with it.
func TestPlanTelemetryCountsOperations(t *testing.T) {
	t.Run("material revision of an approved plan", func(t *testing.T) {
		m := telemetryFixture(t, PlanPending)
		revised := v2Fixture()
		revised.Goal = "a materially different goal"
		_, _, err := m.ReplacePlanV2(revised, false)
		require.NoError(t, err)
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.MaterialRevisions)
		assert.EqualValues(t, 0, s.ApprovalChurn, "the reset itself is not churn")
		assert.EqualValues(t, 0, s.TransitionConflicts)
	})

	t.Run("approval churn counts state flips", func(t *testing.T) {
		m := telemetryFixture(t, PlanPending)
		_, err := m.SetPlanApproved(false)
		require.NoError(t, err)
		_, err = m.SetPlanApproved(true)
		require.NoError(t, err)
		assert.EqualValues(t, 2, m.PlanTelemetry().ApprovalChurn)
		assert.EqualValues(t, 0, m.PlanTelemetry().MaterialRevisions)
	})

	t.Run("refused transition is a conflict", func(t *testing.T) {
		m := telemetryFixture(t, PlanCompleted)
		_, _, err := m.TransitionPlan(transitionPayload(TransitionComplete, "conflict-1"), false)
		require.Error(t, err, "completing an already-completed step must be refused")
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.TransitionConflicts)
		assert.EqualValues(t, 0, s.IdempotentRetries)
	})

	t.Run("replayed mutation is an idempotent retry", func(t *testing.T) {
		m := telemetryFixture(t, PlanInProgress)
		tr := transitionPayload(TransitionComplete, "replay-1")
		_, _, err := m.TransitionPlan(tr, false)
		require.NoError(t, err)
		_, result, err := m.TransitionPlan(tr, false)
		require.NoError(t, err)
		require.True(t, result.Replayed, "the retry must replay, not re-apply")
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.IdempotentRetries)
		assert.EqualValues(t, 0, s.TransitionConflicts)
		assert.EqualValues(t, 0, s.CompletionsWithoutEvidence)
	})

	t.Run("completion without evidence reason counted", func(t *testing.T) {
		m := telemetryFixture(t, PlanInProgress)
		tr := transitionPayload(TransitionComplete, "noev-1")
		tr.Evidence = ""
		tr.EvidenceRefs = nil
		tr.NoEvidenceReason = "nothing observable can exist"
		_, _, err := m.TransitionPlan(tr, false)
		require.NoError(t, err)
		assert.EqualValues(t, 1, m.PlanTelemetry().CompletionsWithoutEvidence)
	})

	t.Run("completion with evidence is not counted", func(t *testing.T) {
		m := telemetryFixture(t, PlanInProgress)
		_, _, err := m.TransitionPlan(transitionPayload(TransitionComplete, "ev-1"), false)
		require.NoError(t, err)
		assert.EqualValues(t, 0, m.PlanTelemetry().CompletionsWithoutEvidence)
	})

	t.Run("plan close records bounded archive latency", func(t *testing.T) {
		m := telemetryFixture(t, PlanInProgress)
		tr := transitionPayload(TransitionComplete, "close-1")
		tr.PlanResult = PlanResultSuccess
		_, result, err := m.TransitionPlan(tr, false)
		require.NoError(t, err)
		require.NotEmpty(t, result.PlanClosed, "completing the last active step closes the plan")
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.Archives)
		assert.Less(t, s.ArchiveLatencyLastMS, uint64(60_000),
			"same-write archiving must land near zero")
		assert.Equal(t, s.ArchiveLatencyLastMS, s.ArchiveLatencyMaxMS)
	})
}

// TestPlanTelemetryCountsSettleOperations extends the operation-to-counter
// contract to the piggyback door: the settle envelope is a public plan
// operation and must be as visible as a plan-tool call.
func TestPlanTelemetryCountsSettleOperations(t *testing.T) {
	t.Run("settle replay is an idempotent retry", func(t *testing.T) {
		m := settleFixture(t)
		_, _, err := m.SettlePlanFromCall(settlePayload("tel-settle-1"))
		require.NoError(t, err)
		_, _, err = m.SettlePlanFromCall(settlePayload("tel-settle-1"))
		require.NoError(t, err)
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.IdempotentRetries)
		assert.EqualValues(t, 0, s.TransitionConflicts)
	})

	t.Run("settle completion without evidence counted", func(t *testing.T) {
		m := settleFixture(t)
		payload := settlePayload("tel-settle-2")
		payload.Complete.Evidence = ""
		payload.Complete.EvidenceRefs = nil
		payload.Complete.NoEvidenceReason = "nothing observable can exist"
		_, _, err := m.SettlePlanFromCall(payload)
		require.NoError(t, err)
		assert.EqualValues(t, 1, m.PlanTelemetry().CompletionsWithoutEvidence)
	})

	t.Run("settle plan close records archive latency", func(t *testing.T) {
		contract := v2Fixture()
		contract.Items = []PlanItem{{
			ID: "solo", Content: "the only active work", Status: PlanInProgress,
			Type: StepEdit, Why: "closing needs no other active step", DoneWhen: "completed",
		}}
		dir := t.TempDir()
		m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
		require.NoError(t, err)
		_, _, err = m.ReplacePlanV2(contract, false)
		require.NoError(t, err)
		_, err = m.SetPlanApproved(true)
		require.NoError(t, err)

		payload := settlePayload("tel-settle-3")
		payload.Complete.StepID = "solo"
		payload.Complete.PlanResult = PlanResultSuccess
		payload.StartStepID = ""
		_, result, err := m.SettlePlanFromCall(payload)
		require.NoError(t, err)
		require.NotEmpty(t, result.Closed, "completing the only step must close the plan")

		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.Archives)
		assert.Less(t, s.ArchiveLatencyLastMS, uint64(60_000), "same-write archiving lands near zero")
	})
}

// TestPlanTelemetryCarriesNoPlanContent is the leak contract at the session
// seam: plan prose, evidence and secret-looking material flow through every
// counted operation, and none of it can appear in the telemetry answer.
func TestPlanTelemetryCarriesNoPlanContent(t *testing.T) {
	// Secret-bearing plan text through the revision door.
	reviser := telemetryFixture(t, PlanPending)
	revised := v2Fixture()
	revised.Goal = "goal with sk-live-TELEMETRYSECRET"
	revised.Items[0].Content = "content with hunter2-password"
	_, _, err := reviser.ReplacePlanV2(revised, false)
	require.NoError(t, err)

	// Secret-bearing evidence through the transition door.
	completer := telemetryFixture(t, PlanInProgress)
	tr := transitionPayload(TransitionComplete, "leak-1")
	tr.Outcome = "outcome with TELEMETRYSECRET"
	tr.Evidence = "evidence with hunter2-password"
	_, _, err = completer.TransitionPlan(tr, false)
	require.NoError(t, err)

	for name, m := range map[string]*Manager{"revision": reviser, "transition": completer} {
		blob, err := json.Marshal(m.PlanTelemetry())
		require.NoError(t, err)
		assert.NotContains(t, string(blob), "TELEMETRYSECRET", name)
		assert.NotContains(t, string(blob), "hunter2", name)
		assert.NotContains(t, string(blob), "alpha", name)
		assert.NotContains(t, string(blob), "goal", name)
	}
}

// TestPlanTelemetryCountsAuthoringFriction extends the operation-to-counter
// contract to the authoring-friction counters: the decision delay, the
// re-decision after a material change, the refused patch, and how the plan
// ended.
func TestPlanTelemetryCountsAuthoringFriction(t *testing.T) {
	t.Run("deciding grant buckets the decision latency", func(t *testing.T) {
		m := telemetryFixture(t, PlanPending)
		before := m.PlanTelemetry().ApprovalLatency1s // the fixture's own approval decided
		_, err := m.SetPlanApproved(false)
		require.NoError(t, err)
		_, err = m.SetPlanApproved(true)
		require.NoError(t, err)
		s := m.PlanTelemetry()
		assert.Equal(t, before+1, s.ApprovalLatency1s, "a same-instant decision lands in the fastest bucket")
		assert.EqualValues(t, 0, s.MaterialReapprovals, "a withdrawal is not a material change")
	})

	t.Run("re-grant after a material change is a reapproval", func(t *testing.T) {
		m := telemetryFixture(t, PlanPending)
		revised := v2Fixture()
		revised.Goal = "a materially different goal"
		_, _, err := m.ReplacePlanV2(revised, false)
		require.NoError(t, err)
		_, err = m.SetPlanApproved(true)
		require.NoError(t, err)
		assert.EqualValues(t, 1, m.PlanTelemetry().MaterialReapprovals)
		// Churn after that decision is not another reapproval.
		_, err = m.SetPlanApproved(false)
		require.NoError(t, err)
		_, err = m.SetPlanApproved(true)
		require.NoError(t, err)
		assert.EqualValues(t, 1, m.PlanTelemetry().MaterialReapprovals)
	})

	t.Run("stale-revision patch refusal is a retry", func(t *testing.T) {
		m := telemetryFixture(t, PlanPending)
		_, _, err := m.PatchPlan(m.Plan().Revision+1, []PlanPatchOp{{Op: PlanPatchUpdateStep}}, false)
		require.Error(t, err)
		assert.EqualValues(t, 1, m.PlanTelemetry().PatchRetries)
	})

	t.Run("plan finish records the outcome", func(t *testing.T) {
		m := telemetryFixture(t, PlanInProgress)
		tr := transitionPayload(TransitionComplete, "friction-finish-1")
		tr.PlanResult = PlanResultSuccess
		_, result, err := m.TransitionPlan(tr, false)
		require.NoError(t, err)
		require.NotEmpty(t, result.PlanClosed)
		s := m.PlanTelemetry()
		assert.EqualValues(t, 1, s.CompletionsSuccess)
		assert.EqualValues(t, 0, s.CompletionsAbandoned)

		abandoned := telemetryFixture(t, PlanInProgress)
		tr = transitionPayload(TransitionComplete, "friction-finish-2")
		tr.PlanResult = PlanResultAbandoned
		_, _, err = abandoned.TransitionPlan(tr, false)
		require.NoError(t, err)
		assert.EqualValues(t, 1, abandoned.PlanTelemetry().CompletionsAbandoned)
	})
}
