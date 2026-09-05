package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetStepModelReplacesAuthoredEffort(t *testing.T) {
	for _, ref := range []string{"plan-b", "plan-b:low", ""} {
		t.Run(ref, func(t *testing.T) {
			ctrl := newReadyController(t)
			contract := patchFixtureContract()
			contract.Items[0].Model = "plan-a:medium"
			contract.Items[0].Effort = "high"
			_, err := ctrl.CreatePlan(t.Context(), contract)
			require.NoError(t, err)
			require.NoError(t, ctrl.SetStepModel("wire-pane", ref))
			item := ctrl.engine.Plan().Items[0]
			require.Equal(t, ref, item.Model)
			require.Empty(t, item.Effort)
		})
	}
}
