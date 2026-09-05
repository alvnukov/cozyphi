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
	// The human chooses the session model; the tool chooses only its effort.
	cfg, ok := resolveOnly(server.URL)("plan-b")
	require.True(t, ok)
	require.NoError(t, engine.SetModel(cfg))

	plan := tools.PlanTool(tools.PlanDeps{
		Create: engine.createPlan,
	})
	_, err := plan.Run(t.Context(), json.RawMessage(`{
		"action":"create","goal":"run at selected depth","approach":"override reasoning effort",
		"successCriteria":["wire request carries effort"],
		"steps":[{"id":"work","content":"change code","type":"edit","why":"needs reasoning",
			"doneWhen":"done","effort":"high"}]
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

func TestIndependentEffortResolvesAfterHumanModelAndRestores(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	for _, pin := range []string{"session", "type", "step"} {
		t.Run(pin, func(t *testing.T) {
			engine := newContextTestEngine(t, server.URL, 100000)
			engine.resolveModel = resolveOnly(server.URL)
			original := engine.ModelConfig()
			original.ReasoningEfforts = []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh}
			original.ReasoningEffort = llm.ReasoningEffortLow
			require.NoError(t, engine.SetModel(original))
			plan := session.Plan{
				Items: []session.PlanItem{
					{ID: "work", Type: session.StepEdit, Status: session.PlanPending, Effort: "high"},
				},
			}
			wantName := original.Name
			if pin == "type" {
				plan.ModelsByType = map[session.StepType]string{session.StepEdit: "plan-b:medium"}
				wantName = "plan-b"
			}
			if pin == "step" {
				plan.ModelsByType = map[session.StepType]string{session.StepEdit: "missing"}
				plan.Items[0].Model = "plan-b:medium"
				wantName = "plan-b"
			}
			require.ErrorContains(t, unstartedAutomationError(plan, "work"), "effort override")
			target, pinned, err := engine.resolveStepModel(plan, "work")
			require.NoError(t, err)
			require.True(t, pinned)
			require.Equal(t, wantName, target.Name)
			require.Equal(t, llm.ReasoningEffortHigh, target.ReasoningEffort)
			require.NoError(t, engine.switchStepModel(target, pinned))
			// A following effort-only step must use the saved session identity.
			plan.ModelsByType = nil
			plan.Items[0].Model = ""
			target, pinned, err = engine.resolveStepModel(plan, "work")
			require.NoError(t, err)
			require.Equal(t, original.Name, target.Name)
			require.NoError(t, engine.switchStepModel(target, pinned))
			engine.restoreSessionModelOnClose()
			require.Equal(t, original, engine.ModelConfig())
			require.NoError(t, engine.switchStepModel(target, pinned))
			require.NoError(t, engine.switchStepModel(llm.ModelConfig{}, false))
			require.Equal(t, original, engine.ModelConfig())
		})
	}
}

func TestIndependentEffortRejectsUnsupportedBeforeStartEffects(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	for _, effort := range []string{"high", "invalid"} {
		engine := newContextTestEngine(t, server.URL, 100000)
		original := engine.ModelConfig()
		plan := session.Plan{Items: []session.PlanItem{{ID: "work", Effort: effort}}}
		err := engine.fireStepStartEffects(t.Context(), plan, "work")
		require.ErrorContains(t, err, "effort")
		require.Equal(t, original, engine.ModelConfig())
		require.False(t, engine.planModelActive)
	}
}
