package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/plangate"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
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

func TestEngineApprovePlanPersistsAndNotifies(t *testing.T) {
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

	_, err = engine.updatePlan(t.Context(), 0, []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	plan, err := engine.SetPlanApproved(true)
	require.NoError(t, err)
	assert.True(t, plan.Approved)
	assert.Equal(t, plan, notified)
	assert.Equal(t, uint64(2), plan.Revision)
}

func TestEngineGateToolListAddsPlanStepToGateableTools(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	list := engine.buildToolList()
	var read, plan, ctx *tools.Tool
	for i := range list {
		switch list[i].Definition.Name {
		case "read":
			read = &list[i]
		case "plan":
			plan = &list[i]
		case "context":
			ctx = &list[i]
		}
	}
	require.NotNil(t, read)
	require.NotNil(t, plan)
	require.NotNil(t, ctx)
	_, ok := read.Definition.Params.Properties["plan_step"]
	assert.True(t, ok)
	_, ok = plan.Definition.Params.Properties["plan_step"]
	assert.False(t, ok)
	_, ok = ctx.Definition.Params.Properties["plan_step"]
	assert.False(t, ok)
}

func TestEnginePlanGatePhaseFollowsMode(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	require.NotNil(t, engine.planGate)
	require.Equal(t, plangate.PhaseHint, engine.planGate.Phase, "build defaults to hint")

	engine.SetMode(ModeUsePlan)
	require.Equal(t, plangate.PhaseDeny, engine.planGate.Phase, "useplan must deny misses")

	engine.SetMode(ModePlan)
	require.Equal(t, plangate.PhaseHint, engine.planGate.Phase, "plan stays hint")

	engine.SetMode(ModeBuild)
	require.Equal(t, plangate.PhaseHint, engine.planGate.Phase, "build stays hint")
}

func TestEngineToolListInjectsPlanStepIntoMetaTools(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{"echo": {Command: []string{"true"}}})
	mgr, err := job.New(job.Options{
		Root:   t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "ok", nil }),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		MCP:         pool,
		Jobs:        mgr,
	})
	require.NoError(t, err)

	list := engine.buildToolList()
	for _, name := range []string{"agent_spawn", "agent_wait", "mcp_list", "mcp_call"} {
		var tool *tools.Tool
		for i := range list {
			if list[i].Definition.Name == name {
				tool = &list[i]
				break
			}
		}
		require.NotNil(t, tool, name)
		_, ok := tool.Definition.Params.Properties["plan_step"]
		assert.True(t, ok, "%s must gain plan_step", name)
	}
}

func TestEnginePromptUsePlanBlocksTool(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	engine.SetMode(ModeUsePlan)
	assert.Contains(t, engine.systemPrompt(), "blocks the tool")

	engine.SetMode(ModeBuild)
	assert.NotContains(t, engine.systemPrompt(), "blocks the tool", "build only hints misses")
}

func TestEnginePromptCarriesPlanGateBlock(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	prompt := engine.systemPrompt()
	assert.Contains(t, prompt, "Plan gate")
	assert.Contains(t, prompt, "plan_step")
	assert.Contains(t, prompt, "explore")
}
