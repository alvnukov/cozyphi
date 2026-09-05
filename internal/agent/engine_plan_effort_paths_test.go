package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestEffortOnlySettleStartAndClose(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	cfg, ok := resolveOnly(server.URL)("plan-b")
	require.True(t, ok)
	require.NoError(t, engine.SetModel(cfg))
	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal:            "use reasoning",
		Approach:        "effort without model selection",
		SuccessCriteria: []string{"wire effort"},
		Items: []session.PlanItem{
			{ID: "work", Content: "work", Type: session.StepEdit, Why: "needed", DoneWhen: "done", Effort: "high"},
		},
	})
	err := engine.settlePlanFromCall(
		t.Context(),
		session.PlanSettle{MutationID: session.NewMutationID(), StartStepID: "work"},
	)
	require.NoError(t, err)
	require.Equal(t, llm.ReasoningEffortHigh, engine.ModelConfig().ReasoningEffort)
	drainLoop(t, engine, "go")
	require.Contains(t, bodies()[0], `"model":"plan-b"`)
	require.Contains(t, bodies()[0], `"reasoning_effort":"high"`)
	err = engine.settlePlanFromCall(
		t.Context(),
		session.PlanSettle{
			MutationID: session.NewMutationID(),
			Complete: &session.PlanTransition{
				Action:     session.TransitionComplete,
				StepID:     "work",
				Outcome:    "done",
				Evidence:   "verified",
				PlanResult: session.PlanResultSuccess,
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, cfg, engine.ModelConfig())
}

func TestLegacyActiveEffortAndReapproval(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	cfg, ok := resolveOnly(server.URL)("plan-b")
	require.True(t, ok)
	require.NoError(t, engine.SetModel(cfg))
	_, err := engine.updatePlan(
		t.Context(),
		[]session.PlanItem{{Content: "work", Type: session.StepEdit, Status: session.PlanInProgress, Effort: "high"}},
	)
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	require.Equal(t, llm.ReasoningEffortHigh, engine.ModelConfig().ReasoningEffort)
	require.Equal(t, cfg.Name, engine.ModelConfig().Name)
	_, err = engine.updatePlan(
		t.Context(),
		[]session.PlanItem{{Content: "work", Type: session.StepEdit, Status: session.PlanCompleted, Effort: "high"}},
	)
	require.NoError(t, err)
	require.Equal(t, cfg, engine.ModelConfig())
}

func TestActiveEffortPatchReapprovalCannotSkipUnsupportedLevel(t *testing.T) {
	server, _, _ := fakeContextServer(t, "", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	cfg, ok := resolveOnly(server.URL)("plan-b")
	require.True(t, ok)
	require.NoError(t, engine.SetModel(cfg))
	seedApprovedActionPlan(
		t,
		engine,
		session.PlanV2{
			Goal:            "work",
			Approach:        "work",
			SuccessCriteria: []string{"done"},
			Items: []session.PlanItem{
				{ID: "work", Content: "work", Type: session.StepEdit, Why: "needed", DoneWhen: "done", Effort: "high"},
			},
		},
	)
	_, _, err := engine.transitionPlan(
		t.Context(),
		session.PlanTransition{Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID()},
	)
	require.NoError(t, err)
	_, _, err = engine.PatchPlan(
		t.Context(),
		engine.Plan().Revision,
		[]session.PlanPatchOp{
			{Op: session.PlanPatchUpdateStep, ID: "work", Effort: session.PatchValue[string]{Set: true, Value: "max"}},
		},
	)
	if err != nil {
		require.ErrorContains(t, err, "effort")
	}
	require.False(t, engine.Plan().Approved)
	require.Equal(t, cfg, engine.ModelConfig(), "rejected override restores the session model")
	_, err = engine.SetPlanApproved(true)
	require.ErrorContains(t, err, "effort")
	require.False(t, engine.Plan().Approved)
}
