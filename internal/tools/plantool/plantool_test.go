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

func TestToolGetsAndUpdatesCanonicalPlan(t *testing.T) {
	current := session.Plan{
		Revision: 2,
		Approved: true,
		Items: []session.PlanItem{{
			Content:  "inspect",
			Status:   session.PlanBlocked,
			Type:     session.StepEdit,
			Note:     "waiting for fixture",
			Evidence: "reproduced with test case",
		}},
	}
	var gotRevision uint64
	var gotItems []session.PlanItem
	plan := plantool.Tool(plantool.Deps{
		Read: func() session.Plan { return current },
		Update: func(_ context.Context, revision uint64, items []session.PlanItem) (session.Plan, error) {
			gotRevision = revision
			gotItems = items
			current = session.Plan{Revision: revision + 1, Approved: true, Items: items}
			return current, nil
		},
	})
	require.Equal(t, "plan", plan.Definition.Name)

	result, err := plan.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"revision":2,
		"approved":true,
		"items":[{
			"content":"inspect",
			"status":"blocked",
			"type":"edit",
			"note":"waiting for fixture",
			"evidence":"reproduced with test case"
		}]
	}`, result.Content)
	assert.Equal(t, "get", plan.DetailFromArgs(json.RawMessage(`{"action":"get"}`)))

	result, err = plan.Run(t.Context(), json.RawMessage(`{
		"action":"update",
		"expected_revision":2,
		"steps":[
			{"content":"inspect","status":"completed","evidence":"targeted test passes"},
			{"content":"implement","status":"in_progress","note":"keep patch local"}
		]
	}`))
	require.NoError(t, err)
	assert.Equal(t, uint64(2), gotRevision)
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
	assert.Equal(t, "update 2 steps", plan.DetailFromArgs(json.RawMessage(`{"action":"update","steps":[{},{}]}`)))
}

func TestToolValidatesActionSpecificParameters(t *testing.T) {
	calls := 0
	plan := plantool.Tool(plantool.Deps{
		Read: func() session.Plan { return session.Plan{} },
		Update: func(_ context.Context, _ uint64, items []session.PlanItem) (session.Plan, error) {
			calls++
			require.NotNil(t, items)
			return session.Plan{Revision: 1, Items: items}, nil
		},
	})

	result, err := plan.Run(t.Context(), json.RawMessage(`{"action":"get"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"revision":0,"items":[]}`, result.Content)

	tests := []struct {
		name string
		args string
	}{
		{name: "missing action", args: `{}`},
		{name: "unknown action", args: `{"action":"replace"}`},
		{name: "get with update revision", args: `{"action":"get","expected_revision":0}`},
		{name: "get with steps", args: `{"action":"get","steps":[]}`},
		{name: "update without revision", args: `{"action":"update","steps":[]}`},
		{name: "update without steps", args: `{"action":"update","expected_revision":0}`},
		{name: "unknown parameter", args: `{"action":"get","extra":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, runErr := plan.Run(t.Context(), json.RawMessage(test.args))
			require.Error(t, runErr)
		})
	}
	assert.Zero(t, calls)

	_, err = plan.Run(t.Context(), json.RawMessage(`{"action":"update","expected_revision":0,"steps":[]}`))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestHintIsConstantSizeAndOmittedWithoutActiveSteps(t *testing.T) {
	assert.Empty(t, plantool.Hint(session.Plan{Revision: 4}))
	hint := plantool.Hint(session.Plan{
		Revision: 5,
		Approved: true,
		Items: []session.PlanItem{
			{Content: "do not inject this", Status: session.PlanBlocked, Note: "nor this"},
			{Content: "done", Status: session.PlanCompleted},
		},
	})
	assert.Contains(t, hint, "revision 5; 2 steps; 1 remaining")
	assert.Contains(t, hint, "approved")
	assert.Contains(t, hint, "plan with action=get")
	assert.NotContains(t, hint, "do not inject this")
	assert.NotContains(t, hint, "nor this")
}
