package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// applyActionsPolicy publishes /settings-style defaults carrying both action
// scopes: one plan-level compact and one inject_skill on the explore type.
func applyActionsPolicy(t *testing.T, engine *Engine) {
	t.Helper()
	defaults := plangate.DefaultDefaults()
	defaults.Actions = []session.PlanAction{{
		Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
	}}
	for i, typ := range defaults.Types {
		if typ.Name == session.StepExplore {
			defaults.Types[i].Actions = []session.PlanAction{{
				Event:  session.PlanActionOnStepStart,
				Type:   session.PlanActionInjectSkill,
				Skills: []string{"tdd"},
			}}
		}
	}
	require.NoError(t, engine.planRuntime.Apply(defaults))
}

func seedContract() session.PlanV2 {
	return session.PlanV2{
		Goal:            "inherit automation",
		Approach:        "defaults seed themselves",
		SuccessCriteria: []string{"actions ride the plan"},
		Items: []session.PlanItem{
			{
				ID: "explore", Type: session.StepExplore, Status: session.PlanPending,
				Content: "read the code", Why: "know the ground", DoneWhen: "code is read",
			},
			{
				ID: "edit", Type: session.StepEdit, Status: session.PlanPending,
				Content: "change the code", Why: "ship it", DoneWhen: "code is changed",
			},
		},
	}
}

// A plan whose author defined no automation inherits the /settings defaults:
// plan-scope actions on the plan, step-scope on items of a configured type.
func TestCreatePlanSeedsDefaultActions(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	applyActionsPolicy(t, engine)

	_, _, err := engine.createPlan(t.Context(), seedContract())
	require.NoError(t, err)

	plan := engine.Plan()
	assert.Equal(t, []session.PlanAction{{
		Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
	}}, plan.Actions)

	explore := plan.Items[0]
	require.Equal(t, "explore", explore.ID)
	assert.Equal(t, []session.PlanAction{{
		Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
		Skills: []string{"tdd"},
	}}, explore.Actions)

	edit := plan.Items[1]
	require.Equal(t, "edit", edit.ID)
	assert.Empty(t, edit.Actions, "types without default actions stay action-free")
}

// An author who wrote actions anywhere wins there: defaults never merge into
// an explicit list, at either scope.
func TestCreatePlanKeepsAuthorActions(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	applyActionsPolicy(t, engine)

	contract := seedContract()
	contract.Actions = []session.PlanAction{{
		Event: session.PlanActionOnPlanEnd, Type: session.PlanActionCompact,
	}}
	contract.Items[0].Actions = []session.PlanAction{{
		Event: session.PlanActionOnStepEnd, Type: session.PlanActionCompact,
	}}
	_, _, err := engine.createPlan(t.Context(), contract)
	require.NoError(t, err)

	plan := engine.Plan()
	assert.Equal(t, contract.Actions, plan.Actions, "plan-scope author list stands")
	assert.Equal(t, contract.Items[0].Actions, plan.Items[0].Actions, "step-scope author list stands")
	assert.Empty(t, plan.Items[1].Actions)
}

// insert_step seeds a fresh step from its type's defaults and leaves authored
// steps alone — the same rule create applies, one step at a time.
func TestPatchPlanInsertStepSeedsDefaultActions(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	applyActionsPolicy(t, engine)
	_, _, err := engine.createPlan(t.Context(), seedContract())
	require.NoError(t, err)

	seeded := &session.PlanItem{
		ID: "explore-more", Type: session.StepExplore, Status: session.PlanPending,
		Content: "read more", Why: "know more", DoneWhen: "more is read",
	}
	authored := &session.PlanItem{
		ID: "run-own", Type: session.StepExplore, Status: session.PlanPending,
		Content: "run own", Why: "author wins", DoneWhen: "own actions stand",
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnStepEnd, Type: session.PlanActionCompact,
		}},
	}
	runStep := &session.PlanItem{
		ID: "run-step", Type: session.StepRun, Status: session.PlanPending,
		Content: "run it", Why: "verify", DoneWhen: "it ran",
	}

	_, summary, err := engine.PatchPlan(t.Context(), engine.Plan().Revision, []session.PlanPatchOp{
		{Op: session.PlanPatchInsertStep, After: "edit", Step: seeded},
		{Op: session.PlanPatchInsertStep, After: "explore-more", Step: authored},
		{Op: session.PlanPatchInsertStep, After: "run-own", Step: runStep},
	})
	require.NoError(t, err)
	require.Len(t, summary.StepsInserted, 3)

	plan := engine.Plan()
	byID := make(map[string]session.PlanItem, len(plan.Items))
	for _, item := range plan.Items {
		byID[item.ID] = item
	}
	assert.Equal(t, []session.PlanAction{{
		Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
		Skills: []string{"tdd"},
	}}, byID["explore-more"].Actions, "inserted explore step inherits its type's defaults")
	assert.Equal(t, authored.Actions, byID["run-own"].Actions, "authored insert keeps its own list")
	assert.Empty(t, byID["run-step"].Actions, "inserted run step stays action-free")
}
