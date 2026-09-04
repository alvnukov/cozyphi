package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
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
