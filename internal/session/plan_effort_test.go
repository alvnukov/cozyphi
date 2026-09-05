package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlanEffortPatchPersistenceAndApproval(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	fixture := actionFixture()
	fixture.Items[1].Effort = " HIGH "
	plan, _, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err)
	require.Equal(t, "high", plan.Items[1].Effort)
	require.Equal(t, "haiku", plan.Items[1].Model)
	plan, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	var ops []PlanPatchOp
	require.NoError(t, json.Unmarshal([]byte(`[{"op":"update_step","id":"decode-legacy","note":"preserve"}]`), &ops))
	plan, _, err = m.PatchPlan(plan.Revision, ops, false)
	require.NoError(t, err)
	require.Equal(t, "high", plan.Items[1].Effort)
	require.True(t, plan.Approved)
	for _, value := range []string{`null`, `"low"`, `""`} {
		require.NoError(
			t,
			json.Unmarshal([]byte(`[{"op":"update_step","id":"decode-legacy","effort":`+value+`}]`), &ops),
		)
		var summary PlanPatchSummary
		plan, summary, err = m.PatchPlan(plan.Revision, ops, false)
		require.NoError(t, err)
		require.False(t, plan.Approved)
		require.Contains(t, summary.Diff[0].Field, "effort")
		require.Equal(t, "haiku", plan.Items[1].Model)
		if value == `"low"` {
			require.Equal(t, "low", plan.Items[1].Effort)
		} else {
			require.Empty(t, plan.Items[1].Effort)
		}
	}
	require.NoError(t, json.Unmarshal([]byte(`[{"op":"update_step","id":"decode-legacy","effort":"high"}]`), &ops))
	plan, _, err = m.PatchPlan(plan.Revision, ops, false)
	require.NoError(t, err)
	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	require.Equal(t, plan.Items, loaded.Plan().Items)
	require.Equal(t, plan.ModelsByType, loaded.Plan().ModelsByType)
	require.NoError(t, json.Unmarshal([]byte(`[{"op":"update_step","id":"decode-legacy","effort":"turbo"}]`), &ops))
	_, _, err = m.PatchPlan(plan.Revision, ops, false)
	require.ErrorContains(t, err, "effort")
	require.Equal(t, plan.Revision, m.Plan().Revision)
}

func TestLegacyPlanEffortSurvivesAndValidates(t *testing.T) {
	m := NewManager(t.TempDir())
	plan, err := m.ReplacePlan([]PlanItem{{Content: "work", Status: PlanPending, Effort: " HIGH "}})
	require.NoError(t, err)
	require.Equal(t, "high", plan.Items[0].Effort)
	_, err = m.ReplacePlan([]PlanItem{{Content: "work", Status: PlanPending, Effort: "turbo"}})
	require.ErrorContains(t, err, "effort")
	require.Equal(t, plan.Revision, m.Plan().Revision)
}
