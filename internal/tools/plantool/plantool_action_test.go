package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

const createAutomationArgs = `{
	"action":"create",
	"goal":"ship the plan automation contract",
	"approach":"actions and models ride the v2 contract",
	"successCriteria":["actions execute on transitions"],
	"steps":[
		{
			"id":"wire-actions","content":"wire the action execution","status":"pending","type":"edit",
			"why":"execution is the core behavior","doneWhen":"engine tests pass",
			"model":"haiku",
			"actions":[
				{"event":"step_start","type":"inject_skill","skills":["tdd"]},
				{"event":"step_end","type":"compact"}
			]
		}
	],
	"actions":[{"event":"plan_start","type":"compact"}],
	"modelsByType":{"edit":"opus","run":"sonnet"}
}`

func TestToolCreatePassesAutomationFields(t *testing.T) {
	var gotContract session.PlanV2
	tool := plantool.Tool(plantool.Deps{
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			gotContract = contract
			return session.Plan{Revision: 2, Schema: session.PlanSchemaV2, Items: contract.Items}, nil, nil
		},
	})

	result, err := tool.Run(t.Context(), json.RawMessage(createAutomationArgs))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"revision":2`)

	require.Len(t, gotContract.Actions, 1)
	assert.Equal(t, session.PlanActionOnPlanStart, gotContract.Actions[0].Event)
	assert.Equal(t, session.PlanActionCompact, gotContract.Actions[0].Type)
	assert.Equal(
		t,
		map[session.StepType]string{session.StepEdit: "opus", session.StepRun: "sonnet"},
		gotContract.ModelsByType,
	)
	require.Len(t, gotContract.Items, 1)
	assert.Equal(t, "haiku", gotContract.Items[0].Model)
	require.Len(t, gotContract.Items[0].Actions, 2)
	assert.Equal(t, session.PlanActionInjectSkill, gotContract.Items[0].Actions[0].Type)
	assert.Equal(t, []string{"tdd"}, gotContract.Items[0].Actions[0].Skills)
	assert.Equal(t, session.PlanActionOnStepEnd, gotContract.Items[0].Actions[1].Event)
}

func TestToolCreateRefusesSeededRuns(t *testing.T) {
	cases := map[string]string{
		"step action runs": `{
			"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d",
				"actions":[{"event":"step_start","type":"compact","runs":[{"status":"ok"}]}]}]}`,
		"plan action runs": `{
			"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d"}],
			"actions":[{"event":"plan_start","type":"compact","runs":[{"status":"ok"}]}]}`,
	}
	tool := plantool.Tool(plantool.Deps{
		Create: func(context.Context, session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			t.Fatal("create must not run for forged run history")
			return session.Plan{}, nil, nil
		},
	})
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "runs")
		})
	}
}

func TestToolRejectsMisroutedAutomationFields(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return session.Plan{}, nil },
		Patch: func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			return session.Plan{}, session.PlanPatchSummary{}, nil
		},
		Transition: func(context.Context, session.PlanTransition) (session.Plan, session.PlanTransitionResult, error) {
			return session.Plan{}, session.PlanTransitionResult{}, nil
		},
	})
	cases := map[string]string{
		"get with actions":      `{"action":"get","actions":[{"event":"plan_start","type":"compact"}]}`,
		"get with modelsByType": `{"action":"get","modelsByType":{"edit":"opus"}}`,
		"patch with actions": `{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s"}],
			"actions":[{"event":"plan_start","type":"compact"}]}`,
		"transition with modelsByType": `{"action":"start","id":"s","mutationId":"m1","modelsByType":{"edit":"opus"}}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.Error(t, err, "automation fields belong to create (or patch ops)")
		})
	}
}

func TestToolUpdateRefusesAutomationOnSteps(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Update: func(context.Context, []session.PlanItem) (session.Plan, error) {
			t.Fatal("legacy update must not run for automation-carrying steps")
			return session.Plan{}, nil
		},
	})
	cases := map[string]string{
		"step actions": `{"action":"update","steps":[{"content":"c","status":"pending",
			"actions":[{"event":"step_start","type":"compact"}]}]}`,
		"step model": `{"action":"update","steps":[{"content":"c","status":"pending","model":"haiku"}]}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.Error(t, err)
		})
	}
}

func TestToolPatchDecodesAutomationOps(t *testing.T) {
	var gotOps []session.PlanPatchOp
	tool := plantool.Tool(plantool.Deps{
		Patch: func(_ context.Context, revision uint64, ops []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			gotOps = ops
			return session.Plan{Revision: revision + 1}, session.PlanPatchSummary{}, nil
		},
	})

	result, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"patch","expected_revision":4,"ops":[
			{"op":"update_step","id":"wire-actions","model":"opus-pro",
			 "actions":[{"event":"step_start","type":"inject_skill","skills":["tdd","code-review"]}]},
			{"op":"set_plan_fields","modelsByType":{"edit":"opus"},"actions":[{"event":"plan_end","type":"compact"}]},
			{"op":"insert_step","after":"wire-actions","step":{
				"id":"new-step","content":"c","type":"edit","why":"w","doneWhen":"d",
				"model":"haiku","actions":[{"event":"step_end","type":"compact"}]}}
		]}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"revision":5`)

	require.Len(t, gotOps, 3)
	require.True(t, gotOps[0].Model.Set)
	assert.Equal(t, "opus-pro", gotOps[0].Model.Value)
	require.True(t, gotOps[0].Actions.Set)
	require.Len(t, gotOps[0].Actions.Value, 1)
	assert.Equal(t, []string{"tdd", "code-review"}, gotOps[0].Actions.Value[0].Skills)
	require.True(t, gotOps[1].ModelsByType.Set)
	assert.Equal(t, map[session.StepType]string{session.StepEdit: "opus"}, gotOps[1].ModelsByType.Value)
	require.True(t, gotOps[1].Actions.Set)
	require.Len(t, gotOps[1].Actions.Value, 1)
	assert.Equal(t, session.PlanActionOnPlanEnd, gotOps[1].Actions.Value[0].Event)
	require.NotNil(t, gotOps[2].Step)
	assert.Equal(t, "haiku", gotOps[2].Step.Model)
	require.Len(t, gotOps[2].Step.Actions, 1)
}

func TestToolPatchRefusesSeededRuns(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Patch: func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			t.Fatal("patch must not run for forged run history")
			return session.Plan{}, session.PlanPatchSummary{}, nil
		},
	})
	_, err := tool.Run(t.Context(), json.RawMessage(`{
		"action":"patch","expected_revision":1,"ops":[
			{"op":"update_step","id":"s",
			 "actions":[{"event":"step_start","type":"compact","runs":[{"status":"ok"}]}]}
		]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs")
}
