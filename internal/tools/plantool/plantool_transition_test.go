package plantool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

const transitionCreateArgs = `{
	"action": "create",
	"goal": "wire transitions through the tool",
	"approach": "semantic actions over stable step ids",
	"successCriteria": ["lifecycle answers with a delta"],
	"steps": [
		{"id": "start-1", "content": "first", "status": "pending", "type": "edit", "why": "w1", "doneWhen": "d1"},
		{"id": "done-2", "content": "second", "status": "completed", "type": "explore", "why": "w2", "doneWhen": "d2"}
	]
}`

func TestToolTransitionAppliesLifecycleThroughRealSession(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)

	_, err := tool.Run(t.Context(), json.RawMessage(transitionCreateArgs))
	require.NoError(t, err)

	result, err := tool.Run(t.Context(), json.RawMessage(
		`{"action":"start","id":"start-1","mutationId":"m-start-1"}`,
	))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"action": "start",
		"stepId": "start-1",
		"from": "pending",
		"to": "in_progress",
		"revision": 2,
		"approved": false
	}`, result.Content)
	assert.Equal(t, "start step start-1", result.Detail)

	result, err = tool.Run(t.Context(), json.RawMessage(`{
		"action": "complete",
		"id": "start-1",
		"mutationId": "m-complete-1",
		"outcome": "shipped",
		"evidence": "focused tests"
	}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"action": "complete",
		"stepId": "start-1",
		"from": "in_progress",
		"to": "completed",
		"revision": 3,
		"approved": false
	}`, result.Content)
	assert.Equal(t, "complete step start-1", result.Detail)

	plan := m.Plan()
	require.Len(t, plan.Events, 2, "each transition leaves one audit event")
	assert.Equal(t, "shipped", plan.Items[0].Outcome)
	assert.Equal(t, "focused tests", plan.Items[0].Evidence)
	assert.False(t, plan.Approved, "finishing the last step closes approval")
}

func TestToolTransitionReplaysRecordedResult(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)
	_, err := tool.Run(t.Context(), json.RawMessage(transitionCreateArgs))
	require.NoError(t, err)

	complete := `{
		"action": "complete",
		"id": "start-1",
		"mutationId": "m-1",
		"outcome": "shipped",
		"evidence": "focused tests"
	}`
	_, err = tool.Run(t.Context(), json.RawMessage(complete))
	require.NoError(t, err)

	result, err := tool.Run(t.Context(), json.RawMessage(complete))
	require.NoError(t, err, "a retried mutation replays instead of failing")
	assert.JSONEq(t, `{
		"action": "complete",
		"stepId": "start-1",
		"from": "pending",
		"to": "completed",
		"revision": 2,
		"approved": false,
		"replayed": true
	}`, result.Content)
	assert.Equal(t, "complete step start-1 (replayed)", result.Detail)

	plan := m.Plan()
	assert.Equal(t, uint64(2), plan.Revision, "a replay moves no revision")
	require.Len(t, plan.Events, 1)
	require.Empty(t, plan.Items[0].EvidenceRefs)
	assert.Equal(t, "focused tests", plan.Items[0].Evidence)
}

func TestToolTransitionPropagatesStateErrors(t *testing.T) {
	deps, _ := managerDeps(t)
	tool := Tool(deps)
	_, err := tool.Run(t.Context(), json.RawMessage(transitionCreateArgs))
	require.NoError(t, err)

	cases := []struct {
		name string
		args string
		want string
	}{
		{
			name: "forbidden action reports allowed moves",
			args: `{"action":"cancel","id":"done-2","mutationId":"m-x","reason":"no longer needed"}`,
			want: `plan transition: session: step "done-2" is completed; allowed actions: reopen`,
		},
		{
			name: "complete requires an outcome",
			args: `{"action":"complete","id":"start-1","mutationId":"m-c","evidence":"proof"}`,
			want: `complete step "start-1": outcome is required`,
		},
		{
			name: "block requires its payload",
			args: `{"action":"block","id":"start-1","mutationId":"m-b","blocker":"upstream"}`,
			want: `block step "start-1": resume_when is required`,
		},
		{
			name: "missing mutation id",
			args: `{"action":"start","id":"start-1"}`,
			want: "plan transition: mutationId is required",
		},
		{
			name: "missing step id",
			args: `{"action":"start","mutationId":"m-s"}`,
			want: "plan transition: id is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(tc.args))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestToolTransitionRejectsMisroutedInput(t *testing.T) {
	deps, _ := managerDeps(t)
	calls := 0
	deps.Transition = func(
		_ context.Context,
		_ session.PlanTransition,
	) (session.Plan, session.PlanTransitionResult, error) {
		calls++
		return session.Plan{}, session.PlanTransitionResult{}, nil
	}
	tool := Tool(deps)

	for _, args := range []string{
		`{"action":"get","mutationId":"m"}`,
		`{"action":"create","reason":"why","steps":[]}`,
		`{"action":"update","blocker":"b","steps":[]}`,
		`{"action":"patch","outcome":"o","expected_revision":1,"ops":[]}`,
		`{"action":"start","id":"a","mutationId":"m","view":"full"}`,
		`{"action":"start","id":"a","mutationId":"m","steps":[]}`,
		`{"action":"start","id":"a","mutationId":"m","ops":[]}`,
		`{"action":"start","id":"a","mutationId":"m","expected_revision":1}`,
		`{"action":"start","id":"a","mutationId":"m","goal":"g"}`,
	} {
		_, err := tool.Run(t.Context(), json.RawMessage(args))
		require.Errorf(t, err, "%s must be refused", args)
	}
	assert.Zero(t, calls, "misrouted input never reaches the session")

	_, err := tool.Run(t.Context(), json.RawMessage(`{"action":"teleport"}`))
	require.ErrorContains(t, err, `unsupported action "teleport"`)
}
