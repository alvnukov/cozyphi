package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

func TestToolCreateRefusesHumanOnlyStepFields(t *testing.T) {
	cases := map[string]string{
		"step model": `{
			"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d",
				"model":"haiku"}]}`,
		"step actions": `{
			"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d",
				"actions":[{"event":"step_start","type":"compact"}]}]}`,
		"step action runs": `{
			"action":"create","goal":"g","approach":"a","successCriteria":["c"],
			"steps":[{"id":"s","content":"c","status":"pending","type":"edit","why":"w","doneWhen":"d",
				"actions":[{"event":"step_start","type":"compact","runs":[{"status":"ok"}]}]}]}`,
	}
	tool := plantool.Tool(plantool.Deps{
		Create: func(context.Context, session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			t.Fatal("create must not run for human-only fields")
			return session.Plan{}, nil, nil
		},
	})
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.ErrorContains(t, err, "human-only")
		})
	}
}

// Plan-level automation fields are gone from the tool schema entirely, so the
// strict decoder refuses them on every action — a stale client naming them
// fails closed before any seam runs.
func TestToolRefusesRemovedPlanLevelFields(t *testing.T) {
	cases := map[string]string{
		"create actions":          `{"action":"create","steps":[],"actions":[]}`,
		"create modelsByType":     `{"action":"create","steps":[],"modelsByType":{"edit":"opus"}}`,
		"get actions":             `{"action":"get","actions":[]}`,
		"get modelsByType":        `{"action":"get","modelsByType":{"edit":"opus"}}`,
		"patch actions":           `{"action":"patch","ops":[],"actions":[]}`,
		"transition modelsByType": `{"action":"start","id":"s","mutationId":"m1","modelsByType":{"edit":"opus"}}`,
	}
	tool := plantool.Tool(plantool.Deps{})
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.ErrorContains(t, err, "unknown field")
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

func TestToolPatchRefusesHumanOnlyOps(t *testing.T) {
	cases := map[string]string{
		"update_step model": `{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s","model":"opus"}]}`,
		"update_step actions": `{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s",
			"actions":[{"event":"step_start","type":"inject_skill","skills":["ghost"]}]}]}`,
		"set_plan_fields modelsByType": `{"action":"patch","expected_revision":1,"ops":[{"op":"set_plan_fields","modelsByType":{"edit":"opus"}}]}`,
		"set_plan_fields actions":      `{"action":"patch","expected_revision":1,"ops":[{"op":"set_plan_fields","actions":[{"event":"plan_end","type":"compact"}]}]}`,
		"insert_step model":            `{"action":"patch","expected_revision":1,"ops":[{"op":"insert_step","step":{"id":"n","content":"c","type":"edit","why":"w","doneWhen":"d","model":"haiku"}}]}`,
		"insert_step actions": `{"action":"patch","expected_revision":1,"ops":[{"op":"insert_step","step":{"id":"n","content":"c","type":"edit","why":"w","doneWhen":"d",
			"actions":[{"event":"step_end","type":"compact","runs":[{"status":"ok"}]}]}}]}`,
	}
	tool := plantool.Tool(plantool.Deps{
		Patch: func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			t.Fatal("patch must not run for human-only fields")
			return session.Plan{}, session.PlanPatchSummary{}, nil
		},
	})
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(args))
			require.ErrorContains(t, err, "human-only")
		})
	}
}
