package plantool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

func TestToolEffortSchemaAndAuthoring(t *testing.T) {
	var items []session.PlanItem
	var ops []session.PlanPatchOp
	calls := 0
	tool := plantool.Tool(plantool.Deps{
		Create: func(_ context.Context, p session.PlanV2) (session.Plan, []session.PlanMaterialChange, []string, error) {
			calls++
			items = p.Items
			return session.Plan{Revision: 1, Schema: session.PlanSchemaV2, Items: items}, nil, nil, nil
		},
		Update: func(_ context.Context, steps []session.PlanItem) (session.Plan, error) {
			calls++
			items = steps
			return session.Plan{Revision: 1, Items: items}, nil
		},
		Patch: func(_ context.Context, _ uint64, patch []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			calls++
			ops = patch
			return session.Plan{Revision: 2, Schema: session.PlanSchemaV2}, session.PlanPatchSummary{}, nil
		},
	})
	raw, err := json.Marshal(tool.Definition.Params)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"model":`)
	assert.Equal(t, 3, strings.Count(string(raw), `"effort":`))
	for _, action := range []string{"create", "update", ""} {
		_, err = tool.Run(
			t.Context(),
			json.RawMessage(
				fmt.Sprintf(
					`{"action":%q,"steps":[{"content":"work","status":"pending","type":"edit","effort":" HIGH "}]}`,
					action,
				),
			),
		)
		require.NoError(t, err)
		assert.Equal(t, "high", items[0].Effort)
	}
	for _, value := range []string{`"high"`, `null`, `""`} {
		_, err = tool.Run(
			t.Context(),
			json.RawMessage(
				`{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s","effort":`+value+`}]}`,
			),
		)
		require.NoError(t, err)
		require.True(t, ops[0].Effort.Set)
		if value == `"high"` {
			assert.Equal(t, "high", ops[0].Effort.Value)
		} else {
			assert.Empty(t, ops[0].Effort.Value)
		}
	}
	_, err = tool.Run(
		t.Context(),
		json.RawMessage(
			`{"action":"patch","expected_revision":1,"ops":[{"op":"update_step","id":"s","note":"keep effort"}]}`,
		),
	)
	require.NoError(t, err)
	assert.False(t, ops[0].Effort.Set)
	before := calls
	for _, payload := range []string{
		`{"steps":[{"effort":"turbo"}]}`,
		`{"action":"create","steps":[{"effort":"turbo"}]}`,
		`{"action":"patch","ops":[{"op":"update_step","effort":"turbo"}]}`,
		`{"action":"patch","ops":[{"op":"insert_step","step":{"effort":"turbo"}}]}`,
		`{"action":"patch","ops":[{"op":"supersede_step","step":{"effort":"turbo"}}]}`,
	} {
		_, err = tool.Run(t.Context(), json.RawMessage(payload))
		require.ErrorContains(t, err, "effort")
	}
	assert.Equal(t, before, calls)
}

func TestToolRejectsActionableModelPresence(t *testing.T) {
	calls := 0
	tool := plantool.Tool(plantool.Deps{
		Create: func(context.Context, session.PlanV2) (session.Plan, []session.PlanMaterialChange, []string, error) {
			calls++
			return session.Plan{}, nil, nil, nil
		},
		Update: func(context.Context, []session.PlanItem) (session.Plan, error) { calls++; return session.Plan{}, nil },
		Patch: func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			calls++
			return session.Plan{}, session.PlanPatchSummary{}, nil
		},
	})
	for _, field := range []string{"model", "Model", "MODEL"} {
		for _, value := range []string{`"other:high"`, `null`, `""`} {
			member := fmt.Sprintf(`%q:%s`, field, value)
			for _, payload := range []string{
				`{"action":"create","steps":[{` + member + `}]}`,
				`{"action":"update","steps":[{` + member + `}]}`,
				`{"steps":[{` + member + `}]}`,
				`{"action":"patch","ops":[{"op":"update_step",` + member + `}]}`,
				`{"action":"patch","ops":[{"op":"insert_step","step":{` + member + `}}]}`,
				`{"action":"patch","ops":[{"op":"update_step","note":"safe"},{"op":"supersede_step","step":{` + member + `}}]}`,
			} {
				_, err := tool.Run(t.Context(), json.RawMessage(payload))
				require.ErrorContains(t, err, "human-only", payload)
			}
		}
	}
	assert.Zero(t, calls)
	// Wrong-action fields remain provider noise rather than authoring intent.
	_, err := tool.Run(
		t.Context(),
		json.RawMessage(`{"action":"update","steps":[],"ops":[{"op":"update_step","model":"ignored"}]}`),
	)
	require.NoError(t, err)
	_, err = tool.Run(
		t.Context(),
		json.RawMessage(
			`{"action":"patch","expected_revision":1,"steps":[{"model":"ignored"}],"ops":[{"op":"remove_step","id":"s","model":"ignored","effort":"ignored","step":{"model":"ignored"}}]}`,
		),
	)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestToolViewsHideHumanModelKeepEffort(t *testing.T) {
	plan := session.Plan{
		Revision:     1,
		Schema:       session.PlanSchemaV2,
		ModelsByType: map[session.StepType]string{session.StepEdit: "private"},
		Items: []session.PlanItem{
			{
				ID:      "s",
				Content: "work",
				Status:  session.PlanPending,
				Type:    session.StepEdit,
				Model:   "private:high",
				Effort:  "low",
			},
		},
	}
	tool := plantool.Tool(plantool.Deps{Get: func(context.Context) (session.Plan, error) { return plan, nil }})
	for _, view := range []string{"active", "full"} {
		result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"`+view+`"}`))
		require.NoError(t, err)
		assert.NotContains(t, result.Content, `"model"`)
		assert.NotContains(t, result.Content, "private")
		assert.Contains(t, result.Content, `"effort":"low"`)
	}
	assert.Equal(t, "private:high", plan.Items[0].Model)
}
