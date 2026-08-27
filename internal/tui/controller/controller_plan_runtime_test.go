package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestControllerReplacementEnginesShareLivePlanRuntime(t *testing.T) {
	ctrl := newReadyController(t)
	runtime := ctrl.PlanRuntime()
	require.NotNil(t, runtime)
	assert.Same(t, runtime, ctrl.engine.PlanRuntime())
	_, err := ctrl.engine.Session().ReplacePlan(t.Context(), []session.PlanItem{{
		Content: "persist session", Status: session.PlanInProgress, Type: session.StepExplore,
	}}, false)
	require.NoError(t, err)
	originalSession := ctrl.SessionID()

	require.NoError(t, runtime.Apply(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "audit", Tools: []string{"read"},
	}}}))
	require.NoError(t, ctrl.Clear())

	assert.Same(t, runtime, ctrl.PlanRuntime())
	assert.Same(t, runtime, ctrl.engine.PlanRuntime())
	assert.Equal(t, []string{"audit"}, ctrl.engine.PlanRuntime().Current().StepTypes())

	_, err = ctrl.Resume(originalSession)
	require.NoError(t, err)
	assert.Same(t, runtime, ctrl.engine.PlanRuntime())
	assert.Equal(t, []string{"audit"}, ctrl.engine.PlanRuntime().Current().StepTypes())
}
