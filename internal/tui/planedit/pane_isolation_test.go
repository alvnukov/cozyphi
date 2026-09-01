package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// isolationPlan builds three clearly distinct steps, so a test can tell from
// the applied patch which step an edit touched.
func isolationPlan() session.Plan {
	plan := fixturePlan()
	plan.Items = []session.PlanItem{
		{
			ID: "alpha", Content: "first content", Type: session.StepExplore,
			Status: session.PlanPending, Why: "alpha why", DoneWhen: "alpha done",
		},
		{
			ID: "beta", Content: "second content", Type: session.StepEdit,
			Status: session.PlanPending, Why: "beta why", DoneWhen: "beta done",
		},
		{
			ID: "gamma", Content: "third content", Type: session.StepRun,
			Status: session.PlanPending, Why: "gamma why", DoneWhen: "gamma done",
		},
	}
	return plan
}

func openStepDetail(t *testing.T, pane *planedit.Pane, id string) {
	t.Helper()
	selectRow(t, pane, id)
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
}

// TestDetailNamesTheStepBeingEdited pins the UX contract that made step
// editing look like "editing every step at once": the detail screen must say
// which step is open, by position and by id, and the step list must show ids.
func TestDetailNamesTheStepBeingEdited(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: isolationPlan()})

	text := renderText(t, pane, 100, 40)
	assert.Contains(t, text, "beta", "the step list shows step ids")
	assert.Contains(t, text, "gamma", "the step list shows step ids")

	openStepDetail(t, pane, "beta")
	detail := renderText(t, pane, 100, 40)
	assert.Contains(t, detail, "Step 2/3", "the detail title names the step position")
	assert.Contains(t, detail, "beta", "the detail screen names the step id")
}

// TestTextEditPopupNamesTheStep: the field editor names the step whose field is
// being edited, so "Edit content" is never anonymous.
func TestTextEditPopupNamesTheStep(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: isolationPlan()})
	openStepDetail(t, pane, "beta")

	selectRow(t, pane, "Content:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	popup := renderText(t, pane, 100, 40)
	assert.Contains(t, popup, "Edit beta · content", "the popup title names step and field")
}

// TestEditingOneStepLeavesOthersUntouched is the behavioral half of the same
// contract: editing one step's field compiles into exactly one update_step
// patch addressed to that step and nothing else.
func TestEditingOneStepLeavesOthersUntouched(t *testing.T) {
	store := &fakeStore{snapshot: isolationPlan()}
	pane := newPane(store)
	openStepDetail(t, pane, "beta")

	selectRow(t, pane, "Note:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	paste(pane, "fresh beta note")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl), "Ctrl+S applies the draft")

	patches := store.applied
	require.Len(t, patches, 1)
	updates := findOps(patches[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1, "only the edited step is patched")
	assert.Equal(t, "beta", updates[0].ID)
	require.True(t, updates[0].Note.Set)
	assert.Equal(t, "fresh beta note", updates[0].Note.Value)

	for _, op := range patches[0].ops {
		assert.NotEqual(t, "alpha", op.ID)
		assert.NotEqual(t, "gamma", op.ID)
	}
	assert.Equal(t, "first content", store.snapshot.Items[0].Content)
	assert.Equal(t, "third content", store.snapshot.Items[2].Content)
}
