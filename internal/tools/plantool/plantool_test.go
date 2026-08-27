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

func TestToolUpdatesCanonicalPlanFromStepsOnly(t *testing.T) {
	var gotItems []session.PlanItem
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			gotItems = items
			return session.Plan{Revision: 3, Approved: true, Items: items}, nil
		},
	})
	require.Equal(t, "plan", plan.Definition.Name)

	result, err := plan.Run(t.Context(), json.RawMessage(`{
		"steps":[
			{"content":"inspect","status":"completed","evidence":"targeted test passes"},
			{"content":"implement","status":"in_progress","note":"keep patch local"}
		]
	}`))
	require.NoError(t, err)
	require.Len(t, gotItems, 2)
	assert.Equal(t, session.PlanInProgress, gotItems[1].Status)
	assert.JSONEq(t, `{
		"revision":3,
		"approved":true,
		"items":[
			{"content":"inspect","status":"completed","evidence":"targeted test passes"},
			{"content":"implement","status":"in_progress","note":"keep patch local"}
		]
	}`, result.Content)
	assert.Equal(t, "update 2 steps", plan.DetailFromArgs(json.RawMessage(`{"steps":[{},{}]}`)))
}

func TestToolDefinitionOnlyRequiresSteps(t *testing.T) {
	definition := plantool.Tool(plantool.Deps{}).Definition
	require.NotNil(t, definition.Params)
	assert.Equal(t, []string{"steps"}, definition.Params.Required)
	_, hasAction := definition.Params.Properties["action"]
	_, hasRevision := definition.Params.Properties["expected_revision"]
	assert.False(t, hasAction)
	assert.False(t, hasRevision)
	assert.Contains(t, definition.Description, "current plan")
}

func TestToolDefinitionUsesConfiguredRequiredStepTypes(t *testing.T) {
	definition := plantool.Tool(plantool.Deps{StepTypes: []string{"inspect", "change"}}).Definition
	raw, err := json.Marshal(definition.Params)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"type":"object",
		"properties":{"steps":{"type":"array","description":"Complete ordered plan snapshot; maximum 32 steps.","maxItems":32,"items":{
			"type":"object",
			"properties":{
				"content":{"type":"string","description":"Specific actionable step; maximum 256 characters.","maxLength":256},
				"status":{"type":"string","enum":["pending","in_progress","blocked","completed","cancelled"]},
				"type":{"type":"string","description":"What this step is allowed to do.","enum":["inspect","change"]},
				"note":{"type":"string","description":"Optional concise finding, assumption, or blocker reason; maximum 256 characters.","maxLength":256},
				"evidence":{"type":"string","description":"Optional concise proof or verification result; maximum 256 characters.","maxLength":256}
			},
			"required":["content","status","type"]
		}}},
		"required":["steps"]
	}`, string(raw))
}

func TestToolToleratesLegacyUpdateMetadataButRejectsGet(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			calls++
			return session.Plan{Revision: 8, Items: items}, nil
		},
	})

	_, err := plan.Run(t.Context(), json.RawMessage(
		`{"action":"update","expected_revision":7,"steps":[]}`,
	))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	_, err = plan.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer supported")
	assert.Equal(t, 1, calls)
}

func TestToolValidatesStepsOnlyInput(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			calls++
			require.NotNil(t, items)
			return session.Plan{Revision: 1, Items: items}, nil
		},
	})

	for _, args := range []string{
		`{}`,
		`{"steps":null}`,
		`{"steps":[],"extra":true}`,
		`{"action":"replace","steps":[]}`,
	} {
		_, err := plan.Run(t.Context(), json.RawMessage(args))
		require.Error(t, err, args)
	}
	assert.Zero(t, calls)

	_, err := plan.Run(t.Context(), json.RawMessage(`{"steps":[]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}
