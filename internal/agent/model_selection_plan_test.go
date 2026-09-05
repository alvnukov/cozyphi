package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSelectModelOverridesActivePlanWithoutRestoringOldDefault(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)
	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "user choice wins", Approach: "select during a pinned step",
		SuccessCriteria: []string{"the user model survives plan close"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "preserve user override", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})
	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	require.Equal(t, "plan-b", engine.ModelStatus().Next.Name)
	cfg := engine.ModelConfig()
	cfg.Name = "user-choice"
	require.Error(t, engine.SelectModel(cfg, llm.ReasoningEffort("ultra")))
	require.Equal(t, "plan-b", engine.ModelStatus().Next.Name)
	require.NoError(t, engine.SelectModel(cfg, llm.ReasoningEffortHigh))
	drainLoop(t, engine, "continue")
	require.Contains(t, bodies()[0], `"model":"user-choice"`)
	require.Contains(t, bodies()[0], `"reasoning_effort":"high"`)
	_, _, err = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "done", Evidence: "code is changed", PlanResult: session.PlanResultSuccess,
	})
	require.NoError(t, err)
	require.Equal(t, "user-choice", engine.ModelStatus().Next.Name)
	require.Equal(t, llm.ReasoningEffortHigh, engine.ModelStatus().Next.Effort)
}
