package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// supersedeStep builds one valid v2 step for the supersede fixtures.
func supersedeStep(id string, status PlanStatus) PlanItem {
	return PlanItem{
		ID:       id,
		Content:  "step " + id,
		Status:   status,
		Type:     StepEdit,
		Why:      "supersede semantics",
		DoneWhen: "recorded",
	}
}

// supersedeOp retires oldID in favor of newID carrying the given capability.
// The replacement restates the retired step's contract verbatim, so only the
// type move crosses the material line the tests pin.
func supersedeOp(oldID, newID string, typ StepType) PlanPatchOp {
	step := supersedeStep(oldID, PlanPending)
	step.ID = newID
	step.Type = typ
	return PlanPatchOp{Op: PlanPatchSupersedeStep, ID: oldID, Step: &step}
}

func TestPatchSupersedeRetiresAndReplaces(t *testing.T) {
	retired := supersedeStep("alpha", PlanInProgress)
	retired.Outcome = "edit concluded"
	retired.Evidence = "focused tests"
	m := finishFixture(t, retired, supersedeStep("beta", PlanCompleted))

	plan, summary, err := m.PatchPlan(2, []PlanPatchOp{supersedeOp("alpha", "alpha-run", StepRun)}, false)
	require.NoError(t, err)

	assert.Equal(t, uint64(3), plan.Revision)
	gone := plan.Items[0]
	assert.Equal(t, PlanSuperseded, gone.Status)
	assert.Equal(t, "alpha-run", gone.SupersededBy)
	assert.Equal(t, "edit concluded", gone.Outcome, "supersede keeps the recorded outcome")
	assert.Equal(t, "focused tests", gone.Evidence, "supersede keeps the recorded evidence")

	replacement := plan.Items[1]
	assert.Equal(t, "alpha-run", replacement.ID)
	assert.Equal(t, PlanPending, replacement.Status)
	assert.Equal(t, StepRun, replacement.Type)
	assert.Equal(t, []string{"alpha->alpha-run"}, summary.StepsSuperseded)
}

func TestPatchSupersedeRefusals(t *testing.T) {
	m := finishFixture(t, supersedeStep("alpha", PlanPending), supersedeStep("beta", PlanCompleted))

	_, _, err := m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchSupersedeStep, ID: "alpha"}}, false)
	require.ErrorContains(t, err, "step is required")

	_, _, err = m.PatchPlan(2, []PlanPatchOp{supersedeOp("ghost", "ghost-run", StepRun)}, false)
	require.ErrorContains(t, err, `step "ghost" not found`)

	foreign := supersedeOp("alpha", "alpha-run", StepRun)
	foreign.Content.Set = true
	foreign.Content.Value = "rewrite"
	_, _, err = m.PatchPlan(2, []PlanPatchOp{foreign}, false)
	require.ErrorContains(t, err, "takes no content")

	_, _, err = m.PatchPlan(2, []PlanPatchOp{supersedeOp("alpha", "alpha-run", StepRun)}, false)
	require.NoError(t, err)
	_, _, err = m.PatchPlan(3, []PlanPatchOp{supersedeOp("alpha", "alpha-again", StepRun)}, false)
	require.ErrorContains(t, err, "already superseded")
}

func TestSupersededStepClosesAsSuccess(t *testing.T) {
	m := finishFixture(t, supersedeStep("alpha", PlanInProgress), supersedeStep("beta", PlanCompleted))
	_, _, err := m.PatchPlan(2, []PlanPatchOp{supersedeOp("alpha", "alpha-run", StepRun)}, false)
	require.NoError(t, err)

	tr := PlanTransition{
		Action:     TransitionComplete,
		StepID:     "alpha-run",
		MutationID: "close-superseded",
		Outcome:    "run concluded",
		Evidence:   "run tests",
		PlanResult: PlanResultSuccess,
	}
	plan, _, err := m.TransitionPlan(tr, false)
	require.NoError(t, err, "a superseded step must not block a success close")
	assert.Equal(t, PlanResultSuccess, plan.Result)
}

func TestPatchSupersedeReapproval(t *testing.T) {
	t.Run("capability change revokes approval", func(t *testing.T) {
		m := finishFixture(t, supersedeStep("alpha", PlanCompleted), supersedeStep("beta", PlanPending))
		plan, summary, err := m.PatchPlan(2, []PlanPatchOp{supersedeOp("alpha", "alpha-run", StepRun)}, false)
		require.NoError(t, err)
		assert.False(t, plan.Approved, "a capability change must revoke approval like any material change")
		require.Len(t, summary.Diff, 1)
		assert.Equal(t, PlanMaterialChange{
			Target: "alpha", Field: "type", Change: MaterialChanged, Detail: "edit to run",
		}, summary.Diff[0])
	})

	t.Run("restated replacement keeps approval", func(t *testing.T) {
		m := finishFixture(t, supersedeStep("alpha", PlanCompleted), supersedeStep("beta", PlanPending))
		plan, summary, err := m.PatchPlan(2, []PlanPatchOp{supersedeOp("alpha", "alpha-next", StepEdit)}, false)
		require.NoError(t, err)
		assert.True(t, plan.Approved, "a supersede that restates the contract is not material")
		assert.Empty(t, summary.Diff)
	})
}

func TestSupersedeLinkInvariant(t *testing.T) {
	replace := func(items ...PlanItem) error {
		contract := v2Fixture()
		contract.Items = items
		dir := t.TempDir()
		m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
		require.NoError(t, err)
		_, _, err = m.ReplacePlanV2(contract, false)
		return err
	}

	t.Run("superseded requires a link", func(t *testing.T) {
		err := replace(supersedeStep("alpha", PlanSuperseded))
		require.ErrorContains(t, err, "superseded without a superseded-by link")
	})

	t.Run("dangling link refuses", func(t *testing.T) {
		orphan := supersedeStep("alpha", PlanSuperseded)
		orphan.SupersededBy = "ghost"
		err := replace(orphan, supersedeStep("beta", PlanPending))
		require.ErrorContains(t, err, "which does not exist")
	})

	t.Run("active step carries no link", func(t *testing.T) {
		linked := supersedeStep("alpha", PlanPending)
		linked.SupersededBy = "beta"
		err := replace(linked, supersedeStep("beta", PlanPending))
		require.ErrorContains(t, err, "carries a superseded-by link")
	})
}
