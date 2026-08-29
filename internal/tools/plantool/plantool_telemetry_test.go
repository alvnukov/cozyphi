package plantool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
)

// secretPlanFixture carries secret-looking plan prose so the leak assertions
// have something real to catch.
func secretPlanFixture() session.Plan {
	plan := v2PlanFixture()
	plan.Goal = "goal with sk-live-PLANVIEWSECRET"
	for i := range plan.Items {
		plan.Items[i].Content = "content with hunter2-password"
	}
	return plan
}

// TestPlanToolTelemetryViewIsBounded pins the diagnostics surface: the
// telemetry view renders the bounded counter snapshot and nothing else — no
// plan prose, no step ids, no secrets from the plan it sits next to.
func TestPlanToolTelemetryViewIsBounded(t *testing.T) {
	snapshot := plantel.Snapshot{
		PlanMisses:          3,
		ProjectionBytesLast: 4321,
	}
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) {
			return secretPlanFixture(), nil
		},
		Telemetry: func(context.Context) (plantel.Snapshot, error) {
			return snapshot, nil
		},
	})
	res, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"telemetry"}`))
	require.NoError(t, err)
	assert.Contains(t, res.Content, "planMisses", "the view renders the snapshot schema")
	assert.Contains(t, res.Content, "4321")
	assert.NotContains(t, res.Content, "PLANVIEWSECRET")
	assert.NotContains(t, res.Content, "hunter2")
	assert.NotContains(t, res.Content, "wire-tool", "step ids are plan content")
}

// TestPlanToolTelemetryViewDegradesWithoutSink pins the degrade contract: a
// tool wired without a telemetry source answers a zero snapshot, not an error
// — telemetry is observability, never a dependency.
func TestPlanToolTelemetryViewDegradesWithoutSink(t *testing.T) {
	tool := plantool.Tool(plantool.Deps{
		Get: func(context.Context) (session.Plan, error) {
			return secretPlanFixture(), nil
		},
	})
	res, err := tool.Run(t.Context(), json.RawMessage(`{"action":"get","view":"telemetry"}`))
	require.NoError(t, err, "no sink configured must degrade, not fail")
	assert.Contains(t, res.Content, "planMisses", "the zero snapshot still renders its schema")
}
