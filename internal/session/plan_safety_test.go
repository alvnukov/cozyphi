package session

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every fixture is a published documentation credential, never a live value.
const (
	secretAWSFixture  = "AKIAIOSFODNN7EXAMPLE"
	secretTokenFixure = "ghp_16C7e42F292c6917E981eE487bD0aA9aBCdefgh"
)

// assertNoSecret plans the whole durable record — items, events, audit — so
// any surface that persists plan text is covered by one assertion.
func assertNoSecret(t *testing.T, plan Plan, secrets ...string) {
	t.Helper()
	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	for _, secret := range secrets {
		assert.NotContains(t, string(encoded), secret)
	}
	assert.Contains(t, string(encoded), "[REDACTED]", "the mask replaces every matched secret")
}

// TestCreateMasksSecretsInModelAuthoredProse: known secret shapes never reach
// the durable snapshot, whatever prose field carried them in.
func TestCreateMasksSecretsInModelAuthoredProse(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	contract := v2Fixture()
	contract.Goal = "ship with " + secretAWSFixture
	contract.WorkingContext = "token " + secretTokenFixure + " in context"
	contract.Items[1].Risk = "leaks " + secretAWSFixture
	plan, _, err := m.ReplacePlanV2(contract, false)
	require.NoError(t, err)

	assertNoSecret(t, plan, secretAWSFixture, secretTokenFixure)
	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	assertNoSecret(t, loaded.Plan(), secretAWSFixture, secretTokenFixure)
}

// TestPatchMasksSecretsInDirectivesAndSteps: the patch door and the receipt it
// returns (material diff details echo directive text) both carry masks only.
func TestPatchMasksSecretsInDirectivesAndSteps(t *testing.T) {
	m := patchedFixture(t)

	patched, summary, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{
		{Op: PlanPatchSetPlanFields, Goal: pv("goal with " + secretAWSFixture)},
		{Op: PlanPatchAddConstraint, Value: "uses " + secretTokenFixure},
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Content: pv("runs with " + secretAWSFixture)},
	}, false)
	require.NoError(t, err)

	assertNoSecret(t, patched, secretAWSFixture, secretTokenFixure)
	for _, change := range summary.Diff {
		assert.NotContains(t, change.Detail, secretAWSFixture)
		assert.NotContains(t, change.Detail, secretTokenFixure)
	}
}

// TestTransitionMasksSecretsInPayloadProse: outcomes, evidence and blockers
// enter the audit trail masked, not only the live item fields.
func TestTransitionMasksSecretsInPayloadProse(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	complete := transitionPayload(TransitionComplete, "m1")
	complete.Outcome = "shipped with " + secretAWSFixture
	complete.Evidence = "token " + secretTokenFixure + " in evidence"
	plan, _, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)

	assertNoSecret(t, plan, secretAWSFixture, secretTokenFixure)
}

// TestRecordPlanAttemptMasksSummarySecrets: the attempt summary is the one
// place raw tool output enters the plan; a secret in its bounded prefix must
// land masked.
func TestRecordPlanAttemptMasksSummarySecrets(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, err := m.RecordPlanAttempt("alpha", PlanAttempt{
		CallID:  "c1",
		Tool:    "bash",
		Status:  AttemptSuccess,
		Summary: "export KEY=" + secretAWSFixture + " ok",
	})
	require.NoError(t, err)
	assertNoSecret(t, plan, secretAWSFixture)
}

// TestSettleMasksSecretsInContextAndOutcome: the piggyback settle door goes
// through the same masking as the plan tool.
func TestSettleMasksSecretsInContextAndOutcome(t *testing.T) {
	m := settleFixture(t)

	payload := settlePayload("settle-mask")
	context := "token " + secretTokenFixure + " in context"
	payload.WorkingContext = &context
	payload.Complete.Outcome = "done with " + secretAWSFixture
	plan, _, err := m.SettlePlanFromCall(payload)
	require.NoError(t, err)

	assertNoSecret(t, plan, secretAWSFixture, secretTokenFixure)
}

// TestWriteDoorsRejectControlCharacters: NUL and C0/DEL control characters
// never persist — they corrupt terminals, logs and diffs. Tabs and newlines
// are legitimate prose and must keep passing.
func TestWriteDoorsRejectControlCharacters(t *testing.T) {
	t.Run("create goal", func(t *testing.T) {
		m := NewManager(t.TempDir())
		contract := v2Fixture()
		contract.Goal = "goal with \x00 nul"
		_, _, err := m.ReplacePlanV2(contract, false)
		assert.ErrorContains(t, err, "control character")
	})

	t.Run("patch step content", func(t *testing.T) {
		m := patchedFixture(t)
		_, _, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{
			{Op: PlanPatchUpdateStep, ID: "decode-legacy", Content: pv("content with \x01")},
		}, false)
		assert.ErrorContains(t, err, "control character")
	})

	t.Run("transition outcome", func(t *testing.T) {
		m := transitionFixture(t, PlanInProgress)
		complete := transitionPayload(TransitionComplete, "m1")
		complete.Outcome = "outcome with \x7f"
		_, _, err := m.TransitionPlan(complete, false)
		assert.ErrorContains(t, err, "control character")
	})

	t.Run("settle working context", func(t *testing.T) {
		m := settleFixture(t)
		payload := settlePayload("settle-ctl")
		context := "context with \x00"
		payload.WorkingContext = &context
		_, _, err := m.SettlePlanFromCall(payload)
		assert.ErrorContains(t, err, "control character")
	})

	t.Run("multiline prose still passes", func(t *testing.T) {
		m := NewManager(t.TempDir())
		contract := v2Fixture()
		contract.Goal = "line one\nline two\tindented"
		_, _, err := m.ReplacePlanV2(contract, false)
		assert.NoError(t, err, "tabs and newlines are legitimate prose")
	})
}

// TestPatchEnforcesSerializedBudget: create refuses oversized snapshots, and
// so must patch — per-field rune caps with wide runes can still multiply past
// the serialized budget, and the patch door is the one that skips the check.
func TestPatchEnforcesSerializedBudget(t *testing.T) {
	m := patchedFixture(t)
	before := m.Plan().Revision

	ops := make([]PlanPatchOp, 0, 30)
	for i := range 30 {
		ops = append(ops, PlanPatchOp{
			Op:    PlanPatchInsertStep,
			After: "decode-legacy",
			Step: &PlanItem{
				ID:       "bulk-" + strconv.Itoa(i),
				Content:  strings.Repeat("🛠", maxPlanContentRunes),
				Type:     StepEdit,
				Why:      strings.Repeat("🛠", maxPlanStepWhyRunes),
				DoneWhen: strings.Repeat("🛠", maxPlanStepDoneWhenRunes),
			},
		})
	}

	_, _, err := m.PatchPlan(before, ops, false)
	require.ErrorContains(t, err, "maximum is", "the patch path enforces the serialized budget")
	assert.Equal(t, before, m.Plan().Revision, "an oversized patch persists nothing")
}
