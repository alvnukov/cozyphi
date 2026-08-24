package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func TestEnginePlanCallbackRunsAfterDurableUpdate(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	got, err := engine.updatePlan(
		t.Context(),
		0,
		[]session.PlanItem{{Content: "verify", Status: session.PlanInProgress}},
	)
	require.NoError(t, err)
	assert.Equal(t, got, notified)
	assert.FileExists(t, engine.SessionFile(), "callback must not run before the plan is durable")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, got.Items, reopened.Plan().Items)
}

func TestEnginePlanCancellationDoesNotMutateOrNotify(t *testing.T) {
	notifications := 0
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		PlanUpdated: func(session.Plan) { notifications++ },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = engine.updatePlan(
		ctx,
		0,
		[]session.PlanItem{{Content: "do not store", Status: session.PlanPending}},
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, notifications)
	assert.Empty(t, engine.Plan().Items)
}

func TestLoopCarriesBoundedPlanHintAcrossResumeWithoutStepText(t *testing.T) {
	server, bodies := capturingTextServer(t)
	dir := t.TempDir()
	model := llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"}

	engine, err := NewEngine(EngineOpts{
		Model: model,
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), 0, []session.PlanItem{{
		Content: "sensitive and potentially very long model-authored step text",
		Status:  session.PlanBlocked,
		Note:    "also model-authored",
	}})
	require.NoError(t, err)

	drainLoop(t, engine, "continue")
	require.Len(t, bodies(), 1)
	first := bodies()[0]
	assert.Contains(t, first, "Current durable plan: revision 1; 1 steps; 1 remaining")
	assert.Contains(t, first, "Call plan with action=get")
	assert.NotContains(t, first, "sensitive and potentially very long")
	assert.NotContains(t, first, "also model-authored")

	resumed, err := NewEngine(EngineOpts{
		Model: model,
		SessionOpts: SessionOpts{
			SessionDir: dir,
			ResumePath: engine.SessionFile(),
		},
	})
	require.NoError(t, err)
	drainLoop(t, resumed, "continue after resume")

	all := bodies()
	require.Len(t, all, 2)
	last := all[1]
	assert.Contains(t, last, "Current durable plan: revision 1; 1 steps; 1 remaining")
	assert.NotContains(t, last, "sensitive and potentially very long")
	assert.NotContains(t, last, "also model-authored")
}
