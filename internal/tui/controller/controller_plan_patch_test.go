package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func patchFixtureContract() session.PlanV2 {
	return session.PlanV2{
		Goal:            "ship the inline plan editor",
		Approach:        "one pane behind a Store seam wired to the durable patch path",
		SuccessCriteria: []string{"apply round-trips through PatchPlan"},
		Constraints:     []string{"no whole-file rewrite of the hashline editor"},
		WorkingContext:  "controller owns the write path",
		Items: []session.PlanItem{{
			ID:       "wire-pane",
			Content:  "wire the pane into the shell",
			Type:     session.StepEdit,
			Status:   session.PlanPending,
			Why:      "a hidden pane is dead code",
			DoneWhen: "Ctrl+P opens the editor",
		}},
	}
}

func TestController_PatchPlanAppliesAndPublishes(t *testing.T) {
	ctrl := newReadyController(t)
	created, _, err := ctrl.engine.Session().ReplacePlanV2(t.Context(), patchFixtureContract(), false)
	require.NoError(t, err)
	_ = ctrl.bus.Drain()

	patched, err := ctrl.PatchPlan(t.Context(), created.Revision, []session.PlanPatchOp{{
		Op:   session.PlanPatchSetPlanFields,
		Goal: session.PatchValue[string]{Set: true, Value: "ship the inline plan editor v2"},
	}})
	require.NoError(t, err)
	assert.Equal(t, created.Revision+1, patched.Revision)
	assert.Equal(t, "ship the inline plan editor v2", patched.Goal)

	var published []session.Plan
	for _, msg := range ctrl.bus.Drain() {
		if planMsg, ok := msg.(PlanUpdatedMsg); ok {
			published = append(published, planMsg.Plan)
		}
	}
	require.Len(t, published, 1, "the patched plan reaches the bus exactly once")
	assert.Equal(t, patched.Revision, published[0].Revision)
}

func TestController_PatchPlanRejectsStaleRevision(t *testing.T) {
	ctrl := newReadyController(t)
	created, _, err := ctrl.engine.Session().ReplacePlanV2(t.Context(), patchFixtureContract(), false)
	require.NoError(t, err)
	_ = ctrl.bus.Drain()

	_, err = ctrl.PatchPlan(t.Context(), created.Revision+9, []session.PlanPatchOp{{
		Op:    session.PlanPatchAddConstraint,
		Value: "too old to land",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revision")
}
