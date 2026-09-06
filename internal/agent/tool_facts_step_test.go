package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// TestAStepTransitionChangesTheReasonsBetweenTwoSnapshots is the "no stale
// cached tool list" contract from the ticket, taken through the mechanism a
// real session uses: the plan moves, and the next observation says something
// different about the same tools. Nothing here calls a tool — the plan is
// moved by the plan tool's own owner and observed afterwards.
func TestAStepTransitionChangesTheReasonsBetweenTwoSnapshots(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)

	unapproved := engine.ToolObservation()
	require.Equal(t, "deny", unapproved.PlanGate)
	assert.NotContains(t, unapproved.Callable, "read",
		"with nothing approved, the plan gate is what stands in the way")

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore,
	}})
	require.NoError(t, err)

	exploring := engine.ToolObservation()
	assert.Contains(t, exploring.Callable, "read", "an approved explore step admits reading")
	assert.NotContains(t, exploring.Callable, "bash", "and no more than reading")
	assert.NotEqual(t, unapproved.Revision, exploring.Revision,
		"the two snapshots describe different reach, and say so")

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "look around", Status: session.PlanCompleted, Type: session.StepExplore},
		{Content: "run it", Status: session.PlanInProgress, Type: session.StepRun},
	})
	require.NoError(t, err)

	running := engine.ToolObservation()
	assert.Contains(t, running.Callable, "bash", "a run step admits the shell")
	assert.NotEqual(t, exploring.Revision, running.Revision)
	assert.Equal(t, running.Registered, exploring.Registered,
		"a step transition changes reach, not registration")
	assert.True(t, engine.HasTool("bash"),
		"and the executor registry kept the full set throughout")
}

// TestObservingTheToolLayerDoesNotMoveThePlan is the other half: the plan the
// observation reads is the plan it leaves behind.
func TestObservingTheToolLayerDoesNotMoveThePlan(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "look around", Status: session.PlanInProgress, Type: session.StepExplore,
	}})
	require.NoError(t, err)
	before := engine.Plan()

	for range 3 {
		engine.ToolObservation()
	}

	assert.Equal(t, before, engine.Plan(),
		"no step is started, completed or approved by looking at the tool layer")
	assert.Equal(t, plangate.PhaseDeny, engine.planGate.Phase,
		"and the gate stands in the phase the useplan posture puts it in")
}
