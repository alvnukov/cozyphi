package sidebar

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSidebarRendersIndependentEffort(t *testing.T) {
	for _, source := range []string{"step", "type", "session", "unset"} {
		t.Run(source, func(t *testing.T) {
			s := visiblePlanSidebar(t)
			s.SetRuntime(Runtime{SessionModel: "base:low"})
			item := session.PlanItem{
				ID:      "work",
				Content: "work",
				Status:  session.PlanPending,
				Type:    session.StepEdit,
				Effort:  "high",
			}
			plan := session.Plan{Revision: 9}
			want := "base"
			switch source {
			case "step":
				item.Model, want = "pinned:low", "pinned"
			case "type":
				plan.ModelsByType = map[session.StepType]string{session.StepEdit: "typed:low"}
				want = "typed"
			case "unset":
				s.SetRuntime(Runtime{})
				want = session.NoModelLabel
			}
			plan.Items = []session.PlanItem{item}
			s.SetPlan(plan)
			for _, width := range []int{40, 56} {
				s.ConfigureWidth(width, nil)
				text := drawWide(s, width)
				require.Contains(t, text, "◇ "+want+" · high", "width %d", width)
				require.NotContains(t, text, "◇ "+want+" · low", "width %d", width)
			}
		})
	}
}
