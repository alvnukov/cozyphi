package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
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
		[]session.PlanItem{{Content: "do not store", Status: session.PlanPending}},
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, notifications)
	assert.Empty(t, engine.Plan().Items)
}

func TestLoopInjectsCurrentPlanWithoutPersistingSnapshot(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content:  "inspect the provider projection",
		Status:   session.PlanInProgress,
		Type:     session.StepExplore,
		Note:     "send this on every inference",
		Evidence: "never persist the synthetic message",
	}})
	require.NoError(t, err)
	plan, err := engine.SetPlanApproved(true)
	require.NoError(t, err)

	drainLoop(t, engine, "continue")
	require.Len(t, bodies(), 1)

	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(bodies()[0]), &request))
	require.NotEmpty(t, request.Messages)
	injected := request.Messages[len(request.Messages)-1]
	assert.Equal(t, string(llm.RoleUser), injected.Role)
	assert.Contains(t, injected.Content, "<current-plan>")
	assert.Contains(t, injected.Content, `"revision":`+fmt.Sprint(plan.Revision))
	assert.Contains(t, injected.Content, `"approved":true`)
	assert.Contains(t, injected.Content, "inspect the provider projection")
	assert.Contains(t, injected.Content, "send this on every inference")
	assert.Contains(t, injected.Content, "never persist the synthetic message")

	for _, message := range engine.session.BuildContext() {
		assert.NotContains(t, message.Content, "<current-plan>", "provider-only context must not enter the session")
	}
}

func TestLoopRefreshesPlanSnapshotAfterUpdateToolRound(t *testing.T) {
	server, bodies := recordingServer(t, func(request int, w http.ResponseWriter) {
		if request == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "plan", `{
				"steps":[{"content":"run the next round","status":"in_progress","type":"run"}]
			}`))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	drain(t, engine, "make a plan")
	sent := bodies()
	require.Len(t, sent, 2)

	contents := make([]string, 0, len(sent))
	for _, body := range sent {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		require.NotEmpty(t, request.Messages)
		contents = append(contents, request.Messages[len(request.Messages)-1].Content)
	}
	assert.Contains(t, contents[0], `"revision":0`)
	assert.Contains(t, contents[0], `"items":[]`)
	assert.NotContains(t, contents[0], "run the next round")
	assert.Contains(t, contents[1], `"revision":1`)
	assert.Contains(t, contents[1], "run the next round")
	assert.Contains(t, contents[1], `"status":"in_progress"`)
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

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	plan, err := engine.SetPlanApproved(true)
	require.NoError(t, err)
	assert.True(t, plan.Approved)
	assert.Equal(t, plan, notified)
	assert.Equal(t, uint64(2), plan.Revision)
}

func TestEngineClearPlanResetsRevisionAndNotifies(t *testing.T) {
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

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	plan, err := engine.ClearPlan()
	require.NoError(t, err)
	assert.Zero(t, plan.Revision, "clear resets the revision counter")
	assert.Empty(t, plan.Items)
	assert.Equal(t, plan, notified, "the republished empty snapshot reaches the subscriber")
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
	require.Equal(t, plangate.PhaseDeny, engine.planGate.Phase, "useplan defaults to deny")

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
