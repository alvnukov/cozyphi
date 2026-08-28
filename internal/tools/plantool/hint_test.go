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

// TestHintIsConstantWhileThePlanIsClosed pins the terminal hint: once the
// contract is discharged the hint stops naming the gate obligation and stays
// byte-identical across closed-plan writes, keeping the prefix cache intact.
func TestHintIsConstantWhileThePlanIsClosed(t *testing.T) {
	base := session.Plan{
		Approved: true, Revision: 7, Result: session.PlanResultSuccess,
		Items: []session.PlanItem{{ID: "s1", Status: session.PlanCompleted, Type: session.StepRun}},
	}
	later := base
	later.Revision = 99
	later.Items = append(
		later.Items,
		session.PlanItem{ID: "s2", Status: session.PlanCancelled, Type: session.StepEdit},
	)

	hint := Hint(base)
	assert.Equal(t, hint, Hint(later), "closed-plan writes must not change the system-prompt bytes")
	assert.Contains(t, hint, "finished", "the hint states the discharged contract")
	assert.NotContains(t, hint, "plan_step", "a closed plan no longer gates calls")
	assert.NotEqual(
		t, hint, Hint(session.Plan{Items: base.Items}), "an open plan keeps the gate obligation",
	)
}
