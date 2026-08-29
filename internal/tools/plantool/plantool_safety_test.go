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

// The description is the contract the model reads; it must forbid the inputs
// the durable plan can never hold and point at evidence refs instead.
func TestToolDefinitionAdvertisesSafetyPolicy(t *testing.T) {
	description := strings.ToLower(plantool.Tool(plantool.Deps{}).Definition.Description)

	for _, phrase := range []string{"secret", "chain-of-thought", "raw logs", "evidence refs"} {
		assert.Contains(t, description, phrase)
	}
	assert.NotContains(
		t,
		description,
		"snapshot also injects",
		"the injected-snapshot claim is stale: the projection rides tool results only",
	)
}

// End to end through the public seams: a plan created by a real session
// manager carries masks, and both get views render the mask, never the secret.
func TestToolViewsMaskSessionSecretsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)

	_, _, err = m.ReplacePlanV2(session.PlanV2{
		Goal:            "ship with AKIAIOSFODNN7EXAMPLE",
		Approach:        "mask at the write funnel",
		SuccessCriteria: []string{"no raw secret in any view"},
		Items: []session.PlanItem{{
			ID: "step", Content: "runs with ghp_16C7e42F292c6917E981eE487bD0aA9aBCdefgh",
			Status: session.PlanPending, Type: session.StepEdit, Why: "w", DoneWhen: "d",
		}},
	}, false)
	require.NoError(t, err)

	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) { return m.Plan(), nil },
	})

	for _, view := range []string{"active", "full"} {
		t.Run(view, func(t *testing.T) {
			result, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"`+view+`"}`))
			require.NoError(t, err)
			assert.NotContains(t, result.Content, "AKIAIOSFODNN7EXAMPLE")
			assert.NotContains(t, result.Content, "ghp_16C7e42F292c6917E981eE487bD0aA9aBCdefgh")
			assert.Contains(t, result.Content, "[REDACTED]")
		})
	}
}
