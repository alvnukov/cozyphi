package planedit

import (
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

var testStepTypes = []session.StepType{session.StepExplore, session.StepEdit, session.StepRun}

func TestDraftDirectiveOpsRevalidateAfterEveryOperation(t *testing.T) {
	t.Run("rename to deleted value", func(t *testing.T) {
		manager, base := newPlanManager(t, []string{"alpha", "beta"}, onePendingStep())
		draft := newDraft(base)
		draft.SuccessCriteria[0].Value = "beta"
		draft.SuccessCriteria = draft.SuccessCriteria[:1]

		patched := applyDraft(t, manager, base, draft)
		assert.Equal(t, []string{"beta"}, patched.SuccessCriteria)
	})

	t.Run("swap", func(t *testing.T) {
		manager, base := newPlanManager(t, []string{"alpha", "beta"}, onePendingStep())
		draft := newDraft(base)
		draft.SuccessCriteria[0].Value = "beta"
		draft.SuccessCriteria[1].Value = "alpha"

		patched := applyDraft(t, manager, base, draft)
		assert.Equal(t, []string{"beta", "alpha"}, patched.SuccessCriteria)
	})

	t.Run("add and delete at cap", func(t *testing.T) {
		criteria := make([]string, maxDirectiveCount)
		for i := range criteria {
			criteria[i] = "criterion-" + strconv.Itoa(i)
		}
		manager, base := newPlanManager(t, criteria, onePendingStep())
		draft := newDraft(base)
		draft.SuccessCriteria = slices.Delete(draft.SuccessCriteria, 0, 1)
		draft.SuccessCriteria = append(draft.SuccessCriteria, directiveDraft{Value: "replacement", New: true})

		patched := applyDraft(t, manager, base, draft)
		require.Len(t, patched.SuccessCriteria, maxDirectiveCount)
		assert.NotContains(t, patched.SuccessCriteria, "criterion-0")
		assert.Contains(t, patched.SuccessCriteria, "replacement")
	})
}

func TestDraftStepOpsFreeCapacityBeforeInsertAndReorderLast(t *testing.T) {
	items := make([]session.PlanItem, 32)
	for i := range items {
		items[i] = pendingStep("step-" + strconv.Itoa(i))
	}
	manager, base := newPlanManager(t, []string{"done"}, items)
	draft := newDraft(base)
	draft.Steps = slices.Delete(draft.Steps, 0, 1)
	added := DraftStep{
		ID: "replacement", Content: "replacement", Type: session.StepEdit, Status: session.PlanPending,
		Why: "capacity stays bounded", DoneWhen: "the replacement exists", JIT: true,
		baseIndex: -1, isNew: true,
	}
	draft.Steps = append([]DraftStep{added}, draft.Steps...)

	ops, err := draft.ops(base, testStepTypes)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	assert.Equal(t, session.PlanPatchRemoveStep, ops[0].Op)
	assert.Equal(t, session.PlanPatchInsertStep, ops[1].Op)
	assert.Equal(t, session.PlanPatchReorderSteps, ops[2].Op)

	patched, _, err := manager.PatchPlan(base.Revision, ops, false)
	require.NoError(t, err)
	require.Len(t, patched.Items, 32)
	assert.Equal(t, "replacement", patched.Items[0].ID)
	assert.True(t, patched.Items[0].JIT)
}

func TestDraftStepOpsRetainAnchorWhileReplacingEveryOldStep(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, []session.PlanItem{
		pendingStep("old-a"), pendingStep("old-b"),
	})
	draft := newDraft(base)
	draft.Steps = []DraftStep{
		newPendingDraftStep("new-a"),
		newPendingDraftStep("new-b"),
	}

	patched := applyDraft(t, manager, base, draft)
	require.Len(t, patched.Items, 2)
	assert.Equal(t, []string{"new-a", "new-b"}, []string{patched.Items[0].ID, patched.Items[1].ID})
}

func TestDraftStepOpsBuildAPlanFromNoSteps(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, nil)
	draft := newDraft(base)
	draft.Steps = []DraftStep{newPendingDraftStep("first"), newPendingDraftStep("second")}

	ops, err := draft.ops(base, testStepTypes)
	require.NoError(t, err)
	require.Len(t, ops, 2, "an empty base needs no reorder: the inserts already land in draft order")
	require.Equal(t, session.PlanPatchInsertStep, ops[0].Op)
	assert.Empty(t, ops[0].Before)
	assert.Empty(t, ops[0].After, "the first step of an empty plan has nothing to anchor to")
	require.Equal(t, session.PlanPatchInsertStep, ops[1].Op)
	assert.Equal(t, "first", ops[1].After, "each later step chains onto the one it follows")

	patched := applyDraft(t, manager, base, draft)
	require.Len(t, patched.Items, 2)
	assert.Equal(t, []string{"first", "second"}, []string{patched.Items[0].ID, patched.Items[1].ID})
}

func TestDraftStepOpsFitAWholePlanAuthoredFromScratch(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, nil)
	draft := newDraft(base)
	for i := range 32 {
		draft.Steps = append(draft.Steps, newPendingDraftStep("step-"+strconv.Itoa(i)))
	}

	patched := applyDraft(t, manager, base, draft)
	require.Len(t, patched.Items, 32, "a plan filled to the step cap in one apply stays inside the op budget")
	assert.Equal(t, "step-0", patched.Items[0].ID)
	assert.Equal(t, "step-31", patched.Items[31].ID)
}

func applyDraft(t *testing.T, manager *session.Manager, base session.Plan, draft Draft) session.Plan {
	t.Helper()
	ops, err := draft.ops(base, testStepTypes)
	require.NoError(t, err)
	require.LessOrEqual(t, len(ops), maxPatchOps)
	patched, _, err := manager.PatchPlan(base.Revision, ops, false)
	require.NoError(t, err)
	return patched
}

func newPlanManager(
	t *testing.T,
	criteria []string,
	items []session.PlanItem,
) (*session.Manager, session.Plan) {
	t.Helper()
	manager := session.NewManager(t.TempDir())
	plan, _, err := manager.ReplacePlanV2(session.PlanV2{
		Goal: "test draft compilation", Approach: "apply through the real manager",
		SuccessCriteria: criteria, Items: items,
	}, false)
	require.NoError(t, err)
	return manager, plan
}

func onePendingStep() []session.PlanItem {
	return []session.PlanItem{pendingStep("anchor")}
}

func pendingStep(id string) session.PlanItem {
	return session.PlanItem{
		ID: id, Content: "do " + id, Type: session.StepEdit, Status: session.PlanPending,
		Why: "the test needs it", DoneWhen: "the test passes",
	}
}

func newPendingDraftStep(id string) DraftStep {
	return DraftStep{
		ID: id, Content: "do " + id, Type: session.StepEdit, Status: session.PlanPending,
		Why: "the replacement is needed", DoneWhen: "the replacement exists",
		baseIndex: -1, isNew: true,
	}
}

func TestDraftDirectiveOpsNeverWriteAValueTheDraftDoesNotHold(t *testing.T) {
	t.Run("swap", func(t *testing.T) {
		manager, base := newPlanManager(t, []string{"alpha", "beta"}, onePendingStep())
		draft := newDraft(base)
		draft.SuccessCriteria[0].Value = "beta"
		draft.SuccessCriteria[1].Value = "alpha"

		ops, err := draft.ops(base, testStepTypes)
		require.NoError(t, err)
		assertOpsWriteOnly(t, ops, []string{"beta", "alpha"})

		patched := applyDraft(t, manager, base, draft)
		assert.Equal(t, []string{"beta", "alpha"}, patched.SuccessCriteria)
	})

	t.Run("three-way rename cycle", func(t *testing.T) {
		manager, base := newPlanManager(t, []string{"alpha", "beta", "gamma"}, onePendingStep())
		draft := newDraft(base)
		draft.SuccessCriteria[0].Value = "beta"
		draft.SuccessCriteria[1].Value = "gamma"
		draft.SuccessCriteria[2].Value = "alpha"

		ops, err := draft.ops(base, testStepTypes)
		require.NoError(t, err)
		assertOpsWriteOnly(t, ops, []string{"beta", "gamma", "alpha"})

		patched := applyDraft(t, manager, base, draft)
		assert.Equal(t, []string{"beta", "gamma", "alpha"}, patched.SuccessCriteria)
	})

	t.Run("cycle among constraints an addition follows", func(t *testing.T) {
		manager, base := newPlanManager(t, []string{"done"}, onePendingStep())
		base, _, err := manager.PatchPlan(base.Revision, []session.PlanPatchOp{
			{Op: session.PlanPatchAddConstraint, Value: "alpha"},
			{Op: session.PlanPatchAddConstraint, Value: "beta"},
		}, false)
		require.NoError(t, err)

		draft := newDraft(base)
		draft.Constraints[0].Value = "beta"
		draft.Constraints[1].Value = "alpha"
		draft.Constraints = append(draft.Constraints, directiveDraft{Value: "gamma", New: true})

		ops, err := draft.ops(base, testStepTypes)
		require.NoError(t, err)
		assertOpsWriteOnly(t, ops, []string{"beta", "alpha", "gamma"})

		patched := applyDraft(t, manager, base, draft)
		assert.Equal(t, []string{"beta", "alpha", "gamma"}, patched.Constraints)
	})
}

// assertOpsWriteOnly pins the rule the compiler exists to keep: an operation
// may name a durable value it deletes or renames away, but every value it
// writes is one the user authored.
func assertOpsWriteOnly(t *testing.T, ops []session.PlanPatchOp, authored []string) {
	t.Helper()
	for i, op := range ops {
		switch op.Op {
		case session.PlanPatchAddCriterion, session.PlanPatchAddConstraint:
			assert.Contains(t, authored, op.Value, "op %d (%s) adds an unauthored value", i+1, op.Op)
		case session.PlanPatchUpdateCriterion, session.PlanPatchUpdateConstraint:
			assert.Contains(t, authored, op.To, "op %d (%s) writes an unauthored value", i+1, op.Op)
		}
	}
}
