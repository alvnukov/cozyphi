package plantool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// managerDeps wires every Deps func onto one real in-memory session manager,
// so the tool contract is exercised against the durable semantics it serves.
func managerDeps(t *testing.T) (Deps, *session.Manager) {
	t.Helper()
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(false))
	require.NoError(t, err)
	return Deps{
		Update: func(_ context.Context, items []session.PlanItem) (session.Plan, error) {
			return m.ReplacePlanWithAutoApprove(items, false)
		},
		Create: func(_ context.Context, contract session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			return m.ReplacePlanV2(contract, false)
		},
		Get: func(context.Context) (session.Plan, error) { return m.Plan(), nil },
		Patch: func(_ context.Context, rev uint64, ops []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			return m.PatchPlan(rev, ops, false)
		},
		Transition: func(_ context.Context, tr session.PlanTransition) (session.Plan, session.PlanTransitionResult, error) {
			return m.TransitionPlan(tr, false)
		},
	}, m
}

const patchCreateArgs = `{
	"action": "create",
	"goal": "wire patch through the tool",
	"approach": "real session behind the deps",
	"successCriteria": ["delta answer"],
	"constraints": ["all-or-none"],
	"workingContext": "bounded",
	"steps": [
		{"id": "done-1", "content": "first", "status": "completed", "type": "explore", "why": "w1", "doneWhen": "d1"},
		{"id": "doing-2", "content": "second", "status": "in_progress", "type": "edit", "why": "w2", "doneWhen": "d2", "risk": "r2"}
	]
}`

const patchOpsArgs = `{
	"action": "patch",
	"expected_revision": 1,
	"ops": [
		{"op": "update_step", "id": "doing-2", "note": "mid-flight"},
		{"op": "insert_step", "after": "doing-2", "step": {"id": "next-3", "content": "third", "type": "run", "why": "w3", "doneWhen": "d3"}},
		{"op": "add_constraint", "value": "bounded batches"},
		{"op": "reorder_steps", "ids": ["next-3", "doing-2", "done-1"]}
	]
}`

func TestToolPatchAppliesOpsThroughRealSession(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)

	_, err := tool.Run(t.Context(), json.RawMessage(patchCreateArgs))
	require.NoError(t, err)

	result, err := tool.Run(t.Context(), json.RawMessage(patchOpsArgs))
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"action": "patch",
		"revision": 2,
		"approved": false,
		"steps": {"total": 3, "remaining": 2},
		"changed": {
			"planFields": ["constraints"],
			"stepsUpdated": ["doing-2"],
			"stepsInserted": ["next-3"],
			"stepsReordered": true,
			"diff": [
				{"target": "plan", "field": "constraints", "change": "added", "detail": "bounded batches"},
				{"target": "next-3", "field": "step", "change": "added"}
			]
		}
	}`, result.Content)
	assert.NotContains(t, result.Content, "successCriteria", "a patch answers with the delta, not the snapshot")
	assert.NotContains(t, result.Content, "approach")
	assert.NotContains(t, result.Content, "workingContext")
	assert.Equal(t, "revision 2, 4 ops, 2 material changes", result.Detail)

	plan := m.Plan()
	assert.Equal(t, []string{"next-3", "doing-2", "done-1"}, []string{
		plan.Items[0].ID, plan.Items[1].ID, plan.Items[2].ID,
	})
	assert.Equal(t, "mid-flight", plan.Items[1].Note)
	assert.Equal(t, session.PlanPending, plan.Items[0].Status, "inserted steps start pending")
	assert.Equal(t, []string{"all-or-none", "bounded batches"}, plan.Constraints)
}

// TestToolCreateReceiptCarriesMaterialDiff pins the create answer's diff
// against the empty plan: every contract field and step reports as material,
// and the one-line detail names the count.
func TestToolCreateReceiptCarriesMaterialDiff(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)

	result, err := tool.Run(t.Context(), json.RawMessage(patchCreateArgs))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"action": "create",
		"revision": 1,
		"approved": false,
		"steps": {"total": 2, "remaining": 1},
		"diff": [
			{"target": "plan", "field": "goal", "change": "changed"},
			{"target": "plan", "field": "approach", "change": "changed"},
			{"target": "plan", "field": "successCriteria", "change": "added", "detail": "delta answer"},
			{"target": "plan", "field": "constraints", "change": "added", "detail": "all-or-none"},
			{"target": "done-1", "field": "step", "change": "added"},
			{"target": "doing-2", "field": "step", "change": "added"}
		]
	}`, result.Content)
	assert.Equal(t, "revision 1, 2 steps, 6 material changes", result.Detail)
	assert.False(t, m.Plan().Approved, "a fresh contract stays a draft")
}

// TestToolPatchReceiptCarriesMaterialDiff pins the patch answer's material
// subset: one risk edit answers with exactly one diff entry and revokes
// approval through the real session.
func TestToolPatchReceiptCarriesMaterialDiff(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)

	_, err := tool.Run(t.Context(), json.RawMessage(patchCreateArgs))
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)

	result, err := tool.Run(t.Context(), json.RawMessage(`{
		"action": "patch",
		"expected_revision": 2,
		"ops": [{"op": "update_step", "id": "doing-2", "risk": "sharper blast radius"}]
	}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"diff":[{"target":"doing-2","field":"risk","change":"changed"}`)
	assert.Contains(t, result.Detail, "1 material change")
	assert.False(t, m.Plan().Approved, "a risk edit revokes the user's approval")
}

// TestToolRefusesSmuggledApproval pins that approval is user-owned: the
// strict input decode has no approved field on any action, so the model
// cannot even send one.
func TestToolRefusesSmuggledApproval(t *testing.T) {
	deps, m := managerDeps(t)
	tool := Tool(deps)

	_, err := tool.Run(t.Context(), json.RawMessage(patchCreateArgs))
	require.NoError(t, err)

	for _, args := range []string{
		`{"action": "create", "approved": true, "goal": "g", "approach": "a",
			"successCriteria": ["c"], "steps": [{"id": "s", "content": "x", "status": "pending", "type": "edit", "why": "w", "doneWhen": "d"}]}`,
		`{"action": "patch", "approved": true, "expected_revision": 1, "ops": [{"op": "replace_context", "workingContext": "c"}]}`,
		`{"action": "start", "approved": true, "id": "doing-2", "mutationId": "m1"}`,
	} {
		_, err := tool.Run(t.Context(), json.RawMessage(args))
		require.ErrorContains(t, err, "approved", "approval is not a model-facing field")
	}
	assert.False(t, m.Plan().Approved, "no path above may leave the plan approved")
}

func TestToolPatchPropagatesConflictWithActualRevision(t *testing.T) {
	deps, _ := managerDeps(t)
	tool := Tool(deps)

	_, err := tool.Run(t.Context(), json.RawMessage(patchCreateArgs))
	require.NoError(t, err)

	_, err = tool.Run(t.Context(), json.RawMessage(`{
		"action": "patch",
		"expected_revision": 99,
		"ops": [{"op": "replace_context", "workingContext": "stale"}]
	}`))
	require.ErrorContains(t, err, "plan patch: session: plan revision is 1; patch expected 99")
}

func TestToolPatchRejectsMisroutedInput(t *testing.T) {
	patches := 0
	deps := Deps{
		Patch: func(_ context.Context, _ uint64, ops []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			patches++
			if len(ops) == 0 {
				return session.Plan{}, session.PlanPatchSummary{}, errors.New("session: plan patch has no operations")
			}
			return session.Plan{}, session.PlanPatchSummary{}, nil
		},
	}
	tool := Tool(deps)

	cases := []struct {
		name string
		args string
		want string
	}{
		{
			"view",
			`{"action": "patch", "view": "full", "expected_revision": 1, "ops": [{"op": "set_plan_fields", "goal": "g"}]}`,
			"plan patch: view is only valid with action get",
		},
		{
			"steps",
			`{"action": "patch", "expected_revision": 1, "steps": []}`,
			"plan patch: takes no steps; use action update or create",
		},
		{
			"top-level contract fields",
			`{"action": "patch", "expected_revision": 1, "goal": "g", "ops": [{"op": "set_plan_fields", "approach": "a"}]}`,
			"plan patch: takes no top-level contract fields; patch ops carry them",
		},
		{
			"missing expected_revision",
			`{"action": "patch", "ops": [{"op": "set_plan_fields", "goal": "g"}]}`,
			"plan patch: expected_revision is required",
		},
		{
			"missing ops",
			`{"action": "patch", "expected_revision": 1}`,
			"plan patch: ops is required",
		},
		{
			"empty ops reach the session",
			`{"action": "patch", "expected_revision": 1, "ops": []}`,
			"plan patch: session: plan patch has no operations",
		},
		{
			"status inside an op is rejected by strict decoding",
			`{"action": "patch", "expected_revision": 1, "ops": [{"op": "update_step", "id": "x", "status": "completed"}]}`,
			`unknown field "status"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(tc.args))
			require.ErrorContains(t, err, tc.want)
		})
	}
	assert.Equal(t, 1, patches, "only the empty-ops case reaches the session")
}
