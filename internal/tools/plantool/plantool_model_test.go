package plantool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

func TestToolModelCatalogBoundsSchemaAndCreate(t *testing.T) {
	var created session.PlanV2
	createCalls := 0
	tool := plantool.Tool(plantool.Deps{
		ModelRefs: []string{" plan-b ", "plan-b:high", "plan-b:high", ""},
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, []string, error) {
			createCalls++
			created = contract
			return session.Plan{Revision: 1, Schema: session.PlanSchemaV2, Items: contract.Items}, nil, nil, nil
		},
	})

	raw, err := json.Marshal(tool.Definition.Params)
	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(string(raw), `"enum":["plan-b","plan-b:high"]`),
		"create, update_step and replacement-step model fields share one bounded catalog")

	valid := `{
		"action":"create","goal":"g","approach":"a","successCriteria":["c"],
		"steps":[{"id":"s","content":"c","type":"edit","why":"w","doneWhen":"d","model":"plan-b:high"}]
	}`
	_, err = tool.Run(t.Context(), json.RawMessage(valid))
	require.NoError(t, err)
	require.Len(t, created.Items, 1)
	assert.Equal(t, "plan-b:high", created.Items[0].Model)
	assert.Equal(t, 1, createCalls)

	invalid := strings.Replace(valid, "plan-b:high", "plan-b:max", 1)
	_, err = tool.Run(t.Context(), json.RawMessage(invalid))
	require.ErrorContains(t, err, "available model catalog")
	assert.Equal(t, 1, createCalls, "an unadvertised reference must fail before storage")
}

func TestToolModelCatalogGuardsPatchAndAllowsClear(t *testing.T) {
	var patched []session.PlanPatchOp
	patchCalls := 0
	tool := plantool.Tool(plantool.Deps{
		ModelRefs: []string{"plan-b", "plan-b:high"},
		Patch: func(_ context.Context, _ uint64, ops []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			patchCalls++
			patched = ops
			return session.Plan{
				Revision: uint64(patchCalls + 1),
				Schema:   session.PlanSchemaV2,
			}, session.PlanPatchSummary{}, nil
		},
	})

	_, err := tool.Run(t.Context(), json.RawMessage(
		`{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s","model":"plan-b:high"}]}`,
	))
	require.NoError(t, err)
	require.Len(t, patched, 1)
	assert.True(t, patched[0].Model.Set)
	assert.Equal(t, "plan-b:high", patched[0].Model.Value)

	_, err = tool.Run(t.Context(), json.RawMessage(
		`{"action":"patch","expected_revision":2,"ops":[{"op":"update_step","id":"s","model":"ghost:high"}]}`,
	))
	require.ErrorContains(t, err, "available model catalog")
	assert.Equal(t, 1, patchCalls)

	_, err = tool.Run(t.Context(), json.RawMessage(
		`{"action":"patch","expected_revision":2,"ops":[{"op":"update_step","id":"s","model":null}]}`,
	))
	require.NoError(t, err)
	assert.True(t, patched[0].Model.Set)
	assert.Empty(t, patched[0].Model.Value)
}

func TestToolViewsKeepPlannerModelReference(t *testing.T) {
	plan := session.Plan{
		Revision: 1,
		Schema:   session.PlanSchemaV2,
		Items: []session.PlanItem{{
			ID: "s", Content: "work", Status: session.PlanPending, Type: session.StepEdit,
			Why: "needed", DoneWhen: "done", Model: "plan-b:high",
		}},
	}
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return plan, nil },
	})

	for _, view := range []string{"active", "full"} {
		result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"`+view+`"}`))
		require.NoError(t, err)
		assert.Contains(t, result.Content, `"model":"plan-b:high"`, view)
	}
}
