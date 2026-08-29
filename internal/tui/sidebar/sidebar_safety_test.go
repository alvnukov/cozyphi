package sidebar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The sidebar renders whatever plan the controller hands it; a plan created
// through the real session door renders the mask, never the raw secret.
func TestSidebarRendersMaskedPlanFromSession(t *testing.T) {
	dir := t.TempDir()
	m, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)

	_, _, err = m.ReplacePlanV2(session.PlanV2{
		Goal:            "ship",
		Approach:        "mask at the write funnel",
		SuccessCriteria: []string{"no raw secret in the sidebar"},
		Items: []session.PlanItem{{
			ID: "step", Content: "runs with AKIAIOSFODNN7EXAMPLE",
			Status: session.PlanInProgress, Type: session.StepEdit, Why: "w", DoneWhen: "d",
		}},
	}, false)
	require.NoError(t, err)

	s := NewSidebar(components.DefaultTheme(), 1000)
	s.SetPlan(m.Plan())
	s.Toggle()

	txt := drawText(s, 40)
	assert.NotContains(t, txt, "AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, txt, "[REDACTED]")
}
