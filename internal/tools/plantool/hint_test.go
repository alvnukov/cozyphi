package plantool

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/session"
)

// TestHintIsConstantAcrossPlanWrites pins the cache contract: the hint rides
// the tail of the system prompt, so a per-write change there breaks the
// provider's prefix cache for the whole conversation history. Plans that
// differ in revision, step count, posture and approval must render the exact
// same bytes; only plan presence may change the hint.
func TestHintIsConstantAcrossPlanWrites(t *testing.T) {
	a := Hint(session.Plan{
		Approved: false,
		Revision: 1,
		Items:    []session.PlanItem{{ID: "s1", Status: session.PlanPending, Type: session.StepRun}},
	})
	b := Hint(session.Plan{
		Approved: true,
		Revision: 42,
		Items: []session.PlanItem{
			{ID: "s1", Status: session.PlanCompleted, Type: session.StepEdit},
			{ID: "s2", Status: session.PlanInProgress, Type: session.StepRun},
			{ID: "s3", Status: session.PlanBlocked, Type: session.StepIntegrate},
		},
	})

	assert.Equal(t, a, b, "plan writes must not change the system-prompt bytes")
	assert.Contains(t, a, "plan_step", "the hint still names the gate obligation")
	assert.Empty(t, Hint(session.Plan{}), "no plan items, no hint")
}
