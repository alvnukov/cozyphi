package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools/plantool"
)

func TestToolReplacesCompletePlan(t *testing.T) {
	var got []session.PlanItem
	tool := plantool.Tool(
		plantool.Deps{Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			got = items
			return session.Plan{Revision: 3, Items: items}, nil
		}},
	)

	result, err := tool.Run(t.Context(), json.RawMessage(`{
		"steps":[
			{"content":"inspect","status":"completed"},
			{"content":"implement","status":"in_progress"}
		]
	}`))
	require.NoError(t, err)
	assert.Equal(t, "update_plan", tool.Definition.Name)
	require.Len(t, got, 2)
	assert.Equal(t, session.PlanInProgress, got[1].Status)
	assert.Contains(t, result.Content, "revision 3")
	assert.Contains(t, result.Content, "1 remaining")
}

func TestToolRequiresStepsButAllowsExplicitClear(t *testing.T) {
	calls := 0
	tool := plantool.Tool(
		plantool.Deps{Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			calls++
			require.NotNil(t, items)
			return session.Plan{Revision: 1, Items: items}, nil
		}},
	)

	_, err := tool.Run(t.Context(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Zero(t, calls)

	_, err = tool.Run(t.Context(), json.RawMessage(`{"steps":[]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}
