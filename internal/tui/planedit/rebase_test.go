package planedit

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestDraftRebaseTakesTheNewerValueForFieldsTheEditorLeftAlone(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, onePendingStep())
	draft := newDraft(base)
	draft.Approach = "the editor rewrote the approach"

	fresh := agentPatch(t, manager, base, session.PlanPatchOp{
		Op: session.PlanPatchSetPlanFields, Goal: patchValue("the agent rewrote the goal"),
	})
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Empty(t, conflicts)
	assert.Equal(t, "the agent rewrote the goal", rebased.Goal)
	assert.Equal(t, "the editor rewrote the approach", rebased.Approach)

	patched := applyDraft(t, manager, fresh, rebased)
	assert.Equal(t, "the agent rewrote the goal", patched.Goal)
	assert.Equal(t, "the editor rewrote the approach", patched.Approach)
}

func TestDraftRebaseKeepsTheNewerValueWhenBothSidesChangedTheSameField(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, onePendingStep())
	draft := newDraft(base)
	draft.Goal = "the editor rewrote the goal"

	fresh := agentPatch(t, manager, base, session.PlanPatchOp{
		Op: session.PlanPatchSetPlanFields, Goal: patchValue("the agent rewrote the goal"),
	})
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Equal(t, []string{"goal"}, conflicts)
	assert.Equal(t, "the agent rewrote the goal", rebased.Goal)

	ops, err := rebased.ops(fresh, testStepTypes)
	require.NoError(t, err)
	assert.Empty(t, ops, "nothing is left to write once the newer value wins")
}

func TestDraftRebaseDropsDirectivesTheNewerPlanNoLongerCarries(t *testing.T) {
	manager, base := newPlanManager(t, []string{"alpha", "beta"}, onePendingStep())
	draft := newDraft(base)
	draft.SuccessCriteria[0].Value = "alpha refined"

	fresh := agentPatch(t, manager, base,
		session.PlanPatchOp{Op: session.PlanPatchRemoveCriterion, Value: "alpha"},
		session.PlanPatchOp{Op: session.PlanPatchAddCriterion, Value: "gamma"},
	)
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Equal(t, []string{`criterion "alpha"`}, conflicts)
	assert.Equal(t, []string{"beta", "gamma"}, directiveValues(rebased.SuccessCriteria))

	ops, err := rebased.ops(fresh, testStepTypes)
	require.NoError(t, err)
	assert.Empty(t, ops, "the added criterion is already durable, and the edited one is gone")
}

func TestDraftRebaseKeepsAnEditOnADirectiveTheAgentLeftAlone(t *testing.T) {
	manager, base := newPlanManager(t, []string{"alpha", "beta"}, onePendingStep())
	draft := newDraft(base)
	draft.SuccessCriteria[1].Value = "beta refined"

	fresh := agentPatch(t, manager, base, session.PlanPatchOp{
		Op: session.PlanPatchRemoveCriterion, Value: "alpha",
	})
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Empty(t, conflicts)
	patched := applyDraft(t, manager, fresh, rebased)
	assert.Equal(t, []string{"beta refined"}, patched.SuccessCriteria)
}

func TestDraftRebaseDropsEditsForAStepTheAgentRemoved(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, []session.PlanItem{
		pendingStep("keep"), pendingStep("drop"),
	})
	draft := newDraft(base)
	draft.Steps[1].Content = "the editor rewrote the doomed step"

	fresh := agentPatch(t, manager, base, session.PlanPatchOp{Op: session.PlanPatchRemoveStep, ID: "drop"})
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Equal(t, []string{`step "drop" was removed from the plan`}, conflicts)
	assert.Equal(t, []string{"keep"}, stepIDs(rebased.Steps))

	ops, err := rebased.ops(fresh, testStepTypes)
	require.NoError(t, err)
	assert.Empty(t, ops)
}

func TestDraftRebaseCancelsADeletionOfAStepThatStarted(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, []session.PlanItem{
		pendingStep("keep"), pendingStep("drop"),
	})
	draft := newDraft(base)
	draft.Steps = slices.Delete(draft.Steps, 1, 2)

	fresh, _, err := manager.TransitionPlan(session.PlanTransition{
		Action: session.TransitionStart, StepID: "drop", MutationID: "started-under-the-modal",
	}, false)
	require.NoError(t, err)

	rebased, conflicts := draft.rebase(base, fresh)

	assert.Equal(t, []string{
		fmt.Sprintf("step %q is %s and can no longer be deleted", "drop", session.PlanInProgress),
	}, conflicts)
	require.Equal(t, []string{"keep", "drop"}, stepIDs(rebased.Steps))
	assert.Equal(t, session.PlanInProgress, rebased.Steps[1].Status)

	ops, err := rebased.ops(fresh, testStepTypes)
	require.NoError(t, err)
	assert.Empty(t, ops, "a step that started is no longer a deletion the patch has to carry")
}

func TestDraftRebaseAdoptsAStepTheAgentInsertedAndRepointsBaseIndexes(t *testing.T) {
	manager, base := newPlanManager(t, []string{"done"}, []session.PlanItem{pendingStep("second")})
	draft := newDraft(base)
	draft.Steps[0].Why = "the editor explained it"

	inserted := pendingStep("first")
	fresh := agentPatch(t, manager, base, session.PlanPatchOp{
		Op: session.PlanPatchInsertStep, Before: "second", Step: &inserted,
	})
	rebased, conflicts := draft.rebase(base, fresh)

	assert.Empty(t, conflicts)
	require.Equal(t, []string{"first", "second"}, stepIDs(rebased.Steps))
	assert.Equal(t, 1, rebased.Steps[1].baseIndex, "the edited step is diffed against its new position")

	patched := applyDraft(t, manager, fresh, rebased)
	require.Len(t, patched.Items, 2)
	assert.Equal(t, "the test needs it", patched.Items[0].Why, "the inserted step is not rewritten")
	assert.Equal(t, "the editor explained it", patched.Items[1].Why)
}

func agentPatch(
	t *testing.T,
	manager *session.Manager,
	base session.Plan,
	ops ...session.PlanPatchOp,
) session.Plan {
	t.Helper()
	fresh, _, err := manager.PatchPlan(base.Revision, ops, false)
	require.NoError(t, err)
	return fresh
}

func directiveValues(entries []directiveDraft) []string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values
}

func stepIDs(steps []DraftStep) []string {
	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	return ids
}
