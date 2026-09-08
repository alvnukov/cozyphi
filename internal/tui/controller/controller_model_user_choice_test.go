package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidebarModelChoiceStillResetsApproval(t *testing.T) {
	ctrl := newReadyController(t)
	contract := patchFixtureContract()
	contract.Items[0].Effort = "high"
	_, err := ctrl.CreatePlan(t.Context(), contract)
	require.NoError(t, err)
	_, err = ctrl.engine.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, ctrl.engine.Plan().Approved)
	require.NoError(t, ctrl.SetStepModel("wire-pane", "plan-b"))
	require.False(t, ctrl.engine.Plan().Approved, "sidebar choice must not inherit editor-save approval preservation")
	require.Equal(t, "plan-b", ctrl.engine.Plan().Items[0].Model)
	require.Empty(t, ctrl.engine.Plan().Items[0].Effort)
}
