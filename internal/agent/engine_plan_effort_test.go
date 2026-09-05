package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// TestStepModelEffortRefAppliedOnStart: a step pinned to "plan-b:high"
// resolves the base model through the config and carries the effort onto
// the wire — the reference convention is one choice, not two fields.
func TestStepModelEffortRefAppliedOnStart(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "run the step on its own depth", Approach: "pin model and effort together",
		SuccessCriteria: []string{"the request names both"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the step is heavier", DoneWhen: "code is changed", Model: "plan-b:high",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`, "the base name must resolve")
	require.Contains(t, bodies()[0], `"reasoning_effort":"high"`, "the pinned effort must reach the wire")
}

// TestStepTypeEffortRefAppliedOnStart: the per-type model map accepts the
// same reference — the effort rides along for unpinned steps too.
func TestStepTypeEffortRefAppliedOnStart(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "cheap but deep exploration", Approach: "a per-type model and effort",
		SuccessCriteria: []string{"explore steps use the mapped pair"},
		ModelsByType:    map[session.StepType]string{session.StepExplore: "plan-b:medium"},
		Items: []session.PlanItem{{
			ID: "scan", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "exploration is bulk work", DoneWhen: "code is read",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "scan", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`)
	require.Contains(t, bodies()[0], `"reasoning_effort":"medium"`)
}

func TestPlanToolAuthoredEffortReachesExecutionRequest(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)
	engine.modelNames = func() []string { return []string{"plan-b"} }

	plan := tools.PlanTool(tools.PlanDeps{
		Create:    engine.createPlan,
		ModelRefs: engine.planModelRefsLocked(),
	})
	_, err := plan.Run(t.Context(), json.RawMessage(`{
		"action":"create","goal":"run at selected depth","approach":"pin one catalog reference",
		"successCriteria":["wire request carries effort"],
		"steps":[{"id":"work","content":"change code","type":"edit","why":"needs reasoning",
			"doneWhen":"done","model":"plan-b:high"}]
	}`))
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	_, _, err = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`)
	require.Contains(t, bodies()[0], `"reasoning_effort":"high"`)
}

func TestPlanModelRefsAdvertiseResolvableModelsAndEfforts(t *testing.T) {
	configs := map[string]llm.ModelConfig{
		"current": {
			Name: "current",
			ReasoningEfforts: []llm.ReasoningEffort{
				llm.ReasoningEffortHigh, llm.ReasoningEffortLow, llm.ReasoningEffortHigh,
			},
		},
		"alpha": {Name: "alpha"},
	}
	engine := &Engine{
		modelCfg:   configs["current"],
		modelNames: func() []string { return []string{"ghost", "alpha", "current"} },
		resolveModel: func(name string) (llm.ModelConfig, bool) {
			cfg, ok := configs[name]
			return cfg, ok
		},
	}

	require.Equal(t, []string{
		"current", "current:low", "current:high", "alpha",
	}, engine.planModelRefsLocked(), "the current executable model stays first and effort levels follow ladder order")
}

func TestPlanModelRefsDoNotAdvertiseUnresolvableModels(t *testing.T) {
	engine := &Engine{
		modelCfg:   llm.ModelConfig{Name: "current", ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortHigh}},
		modelNames: func() []string { return []string{"current"} },
	}
	require.Empty(t, engine.planModelRefsLocked(), "without a resolver no advertised pin could execute")

	engine.resolveModel = func(string) (llm.ModelConfig, bool) { return llm.ModelConfig{}, false }
	require.Empty(t, engine.planModelRefsLocked(), "an unresolved current model must not enter the catalog")
}

func TestPlanModelRefsDoNotCallResolverWithoutCatalog(t *testing.T) {
	calls := 0
	engine := &Engine{
		modelCfg: llm.ModelConfig{Name: "model-default"},
		resolveModel: func(string) (llm.ModelConfig, bool) {
			calls++
			return llm.ModelConfig{}, false
		},
	}

	require.Empty(t, engine.planModelRefsLocked())
	require.Zero(t, calls, "a restore-only resolver is not a planner model catalog")
}

func TestPlanModelRefsKeepResolvableAlias(t *testing.T) {
	engine := &Engine{
		modelCfg:   llm.ModelConfig{Name: "current-alias"},
		modelNames: func() []string { return []string{"current-alias"} },
		resolveModel: func(name string) (llm.ModelConfig, bool) {
			if name != "current-alias" {
				return llm.ModelConfig{}, false
			}
			return llm.ModelConfig{
				Name:             "canonical-name",
				ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortHigh},
			}, true
		},
	}

	require.Equal(t, []string{"current-alias", "current-alias:high"}, engine.planModelRefsLocked())
}
