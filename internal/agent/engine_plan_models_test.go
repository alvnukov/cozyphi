package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// resolveOnly builds a resolver that knows exactly the pinned test model
// "plan-b" and points it at the test server, so a pinned step model still
// serves fake responses.
func resolveOnly(baseURL string) func(string) (llm.ModelConfig, bool) {
	return func(requested string) (llm.ModelConfig, bool) {
		if requested != "plan-b" {
			return llm.ModelConfig{}, false
		}
		return llm.ModelConfig{Name: "plan-b", APIKey: "k", BaseURL: baseURL, ContextWindow: 100000}, true
	}
}

func TestStepModelOverrideAppliedOnStart(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "run the step on its own model", Approach: "pin a model on the step",
		SuccessCriteria: []string{"requests use the pinned model"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the step is heavier", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`, "the step must run on its pinned model")
}

func TestStepModelByTypeAppliedOnStart(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "cheaper exploration", Approach: "a per-type model map",
		SuccessCriteria: []string{"explore steps use the mapped model"},
		ModelsByType:    map[session.StepType]string{session.StepExplore: "plan-b"},
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
	require.Contains(t, bodies()[0], `"model":"plan-b"`, "the type map must pick the model")
}

func TestStepModelOverrideBeatsTypeMap(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = func(requested string) (llm.ModelConfig, bool) {
		if requested != "plan-override" && requested != "plan-type" {
			return llm.ModelConfig{}, false
		}
		return llm.ModelConfig{Name: requested, APIKey: "k", BaseURL: server.URL, ContextWindow: 100000}, true
	}

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "one winner", Approach: "the step pin outranks the type map",
		SuccessCriteria: []string{"the override wins"},
		ModelsByType:    map[session.StepType]string{session.StepEdit: "plan-type"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "precedence needs proof", DoneWhen: "code is changed", Model: "plan-override",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-override"`, "the step override must win")
	require.NotContains(t, bodies()[0], `"model":"plan-type"`)
}

func TestStepModelMissingFromConfigBlocksStart(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "fail closed", Approach: "a model the config cannot name refuses the start",
		SuccessCriteria: []string{"the step stays pending"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "ghost models must not run", DoneWhen: "code is changed", Model: "ghost",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.ErrorContains(t, err, `"ghost"`)
	require.ErrorContains(t, err, "not configured")

	plan := engine.Plan()
	require.Equal(t, session.PlanPending, plan.Items[0].Status, "the step stays pending")
	require.Equal(t, "fake", engine.ModelConfig().Name, "the session model is untouched")
}

func TestSessionModelRestoredOnPlanClose(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "the session survives the plan", Approach: "restore the session model on close",
		SuccessCriteria: []string{"the session model returns"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "step models must not leak", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`)

	_, _, err = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "done", Evidence: "code is changed", PlanResult: session.PlanResultSuccess,
	})
	require.NoError(t, err)
	require.Equal(t, "fake", engine.ModelConfig().Name, "the close restores the session model")

	drainLoop(t, engine, "after the plan")
	require.Contains(t, bodies()[1], `"model":"fake"`, "post-plan turns run on the session model")
}

func TestStepWithoutOverrideRevertsToSessionModel(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "per-step resolution", Approach: "an unpinned step follows the session default",
		SuccessCriteria: []string{"each step resolves its own model"},
		Items: []session.PlanItem{
			{
				ID: "heavy", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
				Why: "this one is heavy", DoneWhen: "code is changed", Model: "plan-b",
			},
			{
				ID: "light", Content: "read the docs", Status: session.PlanPending, Type: session.StepExplore,
				Why: "this one is light", DoneWhen: "docs are read",
			},
		},
	})

	start := func(stepID string) {
		_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
			Action: session.TransitionStart, StepID: stepID, MutationID: session.NewMutationID(),
		})
		require.NoError(t, err)
	}
	complete := func(stepID string) {
		_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
			Action: session.TransitionComplete, StepID: stepID, MutationID: session.NewMutationID(),
			Outcome: "done", Evidence: "step finished",
		})
		require.NoError(t, err)
	}

	start("heavy")
	drainLoop(t, engine, "heavy turn")
	require.Contains(t, bodies()[0], `"model":"plan-b"`)
	complete("heavy")

	start("light")
	drainLoop(t, engine, "light turn")
	require.Contains(t, bodies()[1], `"model":"fake"`, "an unpinned step returns to the session model")
}

func TestStepModelSkippedInUnapprovedDraft(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	_, _, err := engine.createPlan(t.Context(), session.PlanV2{
		Goal: "drafts stay passive", Approach: "automation only after approval",
		SuccessCriteria: []string{"no model switch in a draft"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "drafts do not switch models", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})
	require.NoError(t, err)

	_, _, _ = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.Equal(t, "fake", engine.ModelConfig().Name, "a draft start never switches the model")
}

func TestSettleCloseRestoresSessionModel(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "the envelope closes too", Approach: "the _plan settle restores the session model",
		SuccessCriteria: []string{"settle close restores the model"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the envelope is a door too", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	require.Equal(t, "plan-b", engine.ModelConfig().Name)

	err = engine.settlePlanFromCall(t.Context(), session.PlanSettle{
		MutationID: session.NewMutationID(),
		Complete: &session.PlanTransition{
			Action: session.TransitionComplete, StepID: "work",
			Outcome: "done", Evidence: "code is changed", PlanResult: session.PlanResultSuccess,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "fake", engine.ModelConfig().Name, "a closing settle restores the session model")
}
