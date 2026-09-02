package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

// livePatchDeps wires the tool to a real session manager holding a two-step
// plan, so a rejected patch answers with the session's own message rather than
// a stub's. What the model reads is exactly this text.
func livePatchDeps(t *testing.T) plantool.Deps {
	t.Helper()
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(session.PlanV2{
		Goal:            "ship the patch surface",
		Approach:        "one op at a time",
		SuccessCriteria: []string{"a rejected patch names what is wrong"},
		Items: []session.PlanItem{
			{
				ID: "decode", Content: "decode the wire form", Type: session.StepEdit,
				Why: "the ops arrive as JSON", DoneWhen: "ops decode", Status: session.PlanPending,
			},
			{
				ID: "apply", Content: "apply the batch", Type: session.StepEdit,
				Why: "a batch is all-or-none", DoneWhen: "the batch applies", Status: session.PlanPending,
			},
		},
	}, true)
	require.NoError(t, err)
	return plantool.Deps{
		Patch: func(_ context.Context, rev uint64, ops []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			return m.PatchPlan(rev, ops, true)
		},
		Get: func(context.Context) (session.Plan, error) { return m.Plan(), nil },
	}
}

// A rejected patch must say what is wrong with the patch. The regression it
// guards: these calls used to answer with the transition tool's complaint
// about lifecycle actions, which names nothing the caller sent.
func TestToolPatchRejectionNamesTheOffendingField(t *testing.T) {
	cases := map[string]struct {
		args string
		want string
	}{
		"insert_step without an anchor": {
			args: `{"action":"patch","ops":[{"op":"insert_step","step":{"id":"n","content":"c",` +
				`"type":"edit","why":"w","doneWhen":"d"}}]}`,
			want: "before or after anchor is required",
		},
		"unknown op": {
			args: `{"action":"patch","ops":[{"op":"frobnicate"}]}`,
			want: `unknown op "frobnicate"`,
		},
		"op that sets nothing": {
			args: `{"action":"patch","ops":[{"op":"set_plan_fields"}]}`,
			want: "sets no fields",
		},
		"step id that is not in the plan": {
			args: `{"action":"patch","ops":[{"op":"update_step","id":"ghost","content":"x"}]}`,
			want: "ghost",
		},
	}
	tool := plantool.Tool(livePatchDeps(t))
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.Run(t.Context(), json.RawMessage(tc.args))
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "lifecycle actions",
				"a patch must not be answered with the transition tool's complaint")
		})
	}
}
