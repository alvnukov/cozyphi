package plantool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

const transitionCreateArgs = `{
	"action": "create",
	"goal": "wire transitions through the tool",
	"approach": "semantic actions over stable step ids",
	"successCriteria": ["lifecycle answers with a delta"],
	"steps": [
		{"id": "start-1", "content": "first", "type": "edit", "why": "w1", "doneWhen": "d1"},
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

func TestToolTransitionDerivesRetryIdentityFromToolCall(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)
	_, err := tool.Run(t.Context(), json.RawMessage(transitionCreateArgs))
	require.NoError(t, err)

	ctx := tooldef.WithToolCallID(t.Context(), "toolu_start_1")
	_, err = tool.Run(ctx, json.RawMessage(`{"action":"start","id":"start-1"}`))
	require.NoError(t, err)
	result, err := tool.Run(ctx, json.RawMessage(`{"action":"start","id":"start-1"}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"replayed":true`)
	assert.Len(t, m.Plan().Events, 1, "the same tool call is one durable mutation")
}

// TestToolTransitionScopesSiblingLifecycleDefaults runs every lifecycle action
// with the sibling fields a provider materializes alongside it — non-empty
// wrong-action values, empty arrays, the first enum value, and the other
// actions' top-level defaults — and pins that each call still lands its own
// move. complete is exercised without planResult: an empty materialized string
// must not close the plan.
func TestToolTransitionScopesSiblingLifecycleDefaults(t *testing.T) {
	const create = `{
		"action": "create",
		"goal": "lifecycle under provider noise",
		"approach": "action-authoritative scoping",
		"successCriteria": ["every move lands"],
		"steps": [{"id": "step", "content": "work", "type": "edit", "why": "w", "doneWhen": "d"}]
	}`
	// The noise every case rides along: defaults materialized from the other
	// schema branches, including the first enum values view=active and
	// planResult=success and the empty steps/ops arrays.
	const noise = `"view":"active","expected_revision":0,"steps":[],"ops":[],
		"goal":"","approach":"","successCriteria":[],"constraints":[],
		"workingContext":""`

	cases := []struct {
		name    string
		prepare string // an earlier tool call that puts the step in the from-status
		call    string
		want    session.PlanStatus
	}{
		{
			name: "start ignores the complete and block payloads",
			call: `{"action":"start","id":"step","outcome":"noise","evidence":"noise",
				"evidenceRefs":["noise"],"noEvidenceReason":"noise","blocker":"noise",
				"resumeWhen":"noise","reason":"noise","planResult":"success",` + noise + `}`,
			want: session.PlanInProgress,
		},
		{
			name: "complete ignores the block and cancel payloads and an empty planResult",
			call: `{"action":"complete","id":"step","outcome":"done","evidence":"ran",
				"evidenceRefs":[],"blocker":"noise","resumeWhen":"noise","reason":"noise",` + noise + `}`,
			want: session.PlanCompleted,
		},
		{
			name:    "block ignores the complete and cancel payloads",
			prepare: `{"action":"start","id":"step","mutationId":"m-prep"}`,
			call: `{"action":"block","id":"step","blocker":"waiting on user",
				"resumeWhen":"user answers","outcome":"noise","evidence":"noise",
				"evidenceRefs":["noise"],"reason":"noise","planResult":"success",` + noise + `}`,
			want: session.PlanBlocked,
		},
		{
			name: "resume ignores the payloads it does not own",
			prepare: `{"action":"block","id":"step","mutationId":"m-prep",
				"blocker":"waiting on user","resumeWhen":"user answers"}`,
			call: `{"action":"resume","id":"step","outcome":"noise","evidence":"noise",
				"blocker":"noise","resumeWhen":"noise","reason":"noise",
				"planResult":"success",` + noise + `}`,
			want: session.PlanInProgress,
		},
		{
			name: "cancel ignores the complete and block payloads",
			call: `{"action":"cancel","id":"step","reason":"superseded",
				"outcome":"noise","evidence":"noise","blocker":"noise",
				"resumeWhen":"noise","planResult":"success",` + noise + `}`,
			want: session.PlanCancelled,
		},
		{
			name:    "reopen ignores the complete and block payloads",
			prepare: `{"action":"cancel","id":"step","mutationId":"m-prep","reason":"superseded"}`,
			call: `{"action":"reopen","id":"step","reason":"back in scope",
				"outcome":"noise","evidence":"noise","blocker":"noise",
				"resumeWhen":"noise","planResult":"success",` + noise + `}`,
			want: session.PlanPending,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, m := managerDeps(t)
			tool := Tool(deps)
			_, err := tool.Run(t.Context(), json.RawMessage(create))
			require.NoError(t, err)
			// Harness tool calls carry a stable ID; the noise cases omit
			// mutationId, so identity must come from the call itself.
			ctx := tooldef.WithToolCallID(t.Context(), "call-"+tc.name)
			if tc.prepare != "" {
				prepCtx := tooldef.WithToolCallID(t.Context(), "prep-"+tc.name)
				_, err = tool.Run(prepCtx, json.RawMessage(tc.prepare))
				require.NoError(t, err)
			}

			result, err := tool.Run(ctx, json.RawMessage(tc.call))
			require.NoError(t, err)
			assert.Contains(
				t,
				result.Content,
				`"to":"`+string(tc.want)+`"`,
			) //nolint:gocritic // status string is the wire form

			plan := m.Plan()
			assert.Equal(t, tc.want, plan.Items[0].Status, "the move landed")
			assert.Empty(t, plan.Result, "materialized planResult noise never closes the plan")
		})
	}
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

func TestToolTransitionScopesKnownForeignFieldsToTheSelectedAction(t *testing.T) {
	calls := 0
	deps := Deps{
		Transition: func(
			_ context.Context,
			_ session.PlanTransition,
		) (session.Plan, session.PlanTransitionResult, error) {
			calls++
			return session.Plan{}, session.PlanTransitionResult{}, nil
		},
	}
	tool := Tool(deps)

	for _, args := range []string{
		`{"action":"start","id":"a","mutationId":"m1","view":"full"}`,
		`{"action":"start","id":"a","mutationId":"m2","steps":[]}`,
		`{"action":"start","id":"a","mutationId":"m3","ops":[]}`,
		`{"action":"start","id":"a","mutationId":"m4","expected_revision":1}`,
		`{"action":"start","id":"a","mutationId":"m5","goal":"g"}`,
	} {
		_, err := tool.Run(t.Context(), json.RawMessage(args))
		require.NoErrorf(t, err, "%s must be normalized", args)
	}
	assert.Equal(t, 5, calls)

	_, err := tool.Run(t.Context(), json.RawMessage(`{"action":"teleport"}`))
	require.ErrorContains(t, err, `unsupported action "teleport"`)
}
