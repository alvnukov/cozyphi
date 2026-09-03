package plantool

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/session"
)

// TestModelVisibleDiffKeepsSkillChanges: the receipt hides the user's
// automation choices, but a skills change is the model's own — the
// inject_skill line stays so the receipt reports what moved.
func TestModelVisibleDiffKeepsSkillChanges(t *testing.T) {
	visible := modelVisibleDiff([]session.PlanMaterialChange{
		{Target: "s", Field: "actions", Detail: "1: step_start inject_skill (off: none to tdd)"},
		{Target: "s", Field: "actions", Detail: "2: step_end compact"},
		{Target: "plan", Field: "modelsByType", Detail: "edit: sonnet"},
	})
	assert.Equal(t, []session.PlanMaterialChange{
		{Target: "s", Field: "actions", Detail: "1: step_start inject_skill (off: none to tdd)"},
	}, visible)
}
