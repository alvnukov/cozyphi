package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestController_UserSavePreservesApproval(t *testing.T) {
	for _, approved := range []bool{false, true} {
		name := "draft"
		if approved {
			name = "approved"
		}
		t.Run(name, func(t *testing.T) {
			ctrl := newReadyController(t)
			created, _, _, err := ctrl.engine.Session().ReplacePlanV2(t.Context(), patchFixtureContract(), approved)
			require.NoError(t, err)
			require.Equal(t, approved, created.Approved)
			_ = ctrl.bus.Drain()
			patched, err := ctrl.PatchPlan(t.Context(), created.Revision, []session.PlanPatchOp{{
				Op:   session.PlanPatchSetPlanFields,
				Goal: session.PatchValue[string]{Set: true, Value: "save the user-revised plan"},
			}})
			require.NoError(t, err)
			assert.Equal(t, approved, patched.Approved)
			assert.Equal(t, created.Revision+1, patched.Revision)
			assert.Equal(t, approved, ctrl.engine.Session().Plan().Approved)
			var published []session.Plan
			for _, msg := range ctrl.bus.Drain() {
				if update, ok := msg.(PlanUpdatedMsg); ok {
					published = append(published, update.Plan)
				}
			}
			require.Len(t, published, 1)
			assert.Equal(t, approved, published[0].Approved)
		})
	}
}

func TestController_UserSaveDoesNotBypassAgentReapproval(t *testing.T) {
	ctrl := newReadyController(t)
	created, _, _, err := ctrl.engine.Session().ReplacePlanV2(t.Context(), patchFixtureContract(), true)
	require.NoError(t, err)
	ops := []session.PlanPatchOp{
		{Op: session.PlanPatchSetPlanFields, Goal: session.PatchValue[string]{Set: true, Value: "human revision"}},
	}
	saved, err := ctrl.PatchPlan(t.Context(), created.Revision, ops)
	require.NoError(t, err)
	require.True(t, saved.Approved)
	assert.Greater(t, saved.ContractEpoch, created.ContractEpoch, "material user edits must still expire JIT grants")
	_ = ctrl.bus.Drain()
	_, err = ctrl.PatchPlan(t.Context(), created.Revision, ops)
	require.Error(t, err)
	assert.Equal(t, saved.Revision, ctrl.engine.Plan().Revision)
	assert.True(t, ctrl.engine.Plan().Approved)
	assert.Empty(t, ctrl.bus.Drain(), "a stale save must not publish")
	ops[0].Goal.Value = "agent revision"
	patched, _, err := ctrl.engine.PatchPlan(t.Context(), saved.Revision, ops)
	require.NoError(t, err)
	assert.False(t, patched.Approved, "the model path must still invalidate approval")
}
