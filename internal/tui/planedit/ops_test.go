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

func TestDraftStepOpsExplainWhyAnEmptyBaseCannotAnchorInsert(t *testing.T) {
	_, base := newPlanManager(t, []string{"done"}, nil)
	draft := newDraft(base)
	draft.Steps = []DraftStep{newPendingDraftStep("first")}

	_, err := draft.ops(base, testStepTypes)
	require.ErrorContains(t, err, "insert_step requires an existing step anchor")
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
