package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// skillStore seeds the pending step with an inject_skill action carrying a
// user off mark: tdd live, grill disabled from the sidebar.
func skillStore() *fakeStore {
	plan := fixturePlan()
	plan.Items[1].Actions = []session.PlanAction{{
		Event:          session.PlanActionOnStepEnd,
		Type:           session.PlanActionInjectSkill,
		Skills:         []string{"tdd", "grill"},
		DisabledSkills: []string{"grill"},
	}}
	return &fakeStore{
		snapshot: plan,
		types:    []session.StepType{session.StepExplore, session.StepEdit, session.StepRun},
		models:   []string{"plan-a"},
	}
}

// clearTextField wipes a pre-populated text field with backspaces so a
// paste replaces the value instead of appending to it.
func clearTextField(t *testing.T, pane *planedit.Pane) {
	t.Helper()
	for range 128 {
		key(pane, xui.KeyBackspace, 0, 0)
	}
}

// savePendingStep opens the pending step's detail and saves, returning the
// actions the emitted update carried.
func savePendingStep(t *testing.T, pane *planedit.Pane, store *fakeStore) []session.PlanAction {
	t.Helper()
	openPendingStepDetail(t, pane)
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	require.True(t, updates[0].Actions.Set)
	return updates[0].Actions.Value
}

func TestStepSkillsRowShowsOffMarks(t *testing.T) {
	pane := newPane(skillStore())
	openPendingStepDetail(t, pane)

	assert.Contains(t, renderText(t, pane, 100, 40), "skills: tdd, grill (off)",
		"the row reads the effective set with the user's off mark")
}

func TestAuthoringKeepsOffMarksThroughActionEdit(t *testing.T) {
	store := skillStore()
	pane := newPane(store)
	openPendingStepDetail(t, pane)

	// Re-author the action's event without touching the skills list: the
	// emitted actions must still carry the user's off mark.
	selectRow(t, pane, "⚙ 1 event:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	// The current event (step_end) is preselected last; one Up picks step_start.
	require.True(t, key(pane, xui.KeyUp, 0, 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	require.True(t, updates[0].Actions.Set)
	emitted := updates[0].Actions.Value
	require.Len(t, emitted, 1)
	assert.Equal(t, session.PlanActionOnStepStart, emitted[0].Event)
	assert.Equal(t, []string{"tdd", "grill"}, emitted[0].Skills)
	assert.Equal(t, []string{"grill"}, emitted[0].DisabledSkills,
		"an action edit that never touches skills keeps the toggle")
}

func TestAuthoringKeepsOffMarksOnlyForSurvivingSkills(t *testing.T) {
	t.Run("surviving name keeps its off mark", func(t *testing.T) {
		store := skillStore()
		pane := newPane(store)
		openPendingStepDetail(t, pane)

		selectRow(t, pane, "⚙ 1 inject_skill · skills")
		require.True(t, key(pane, xui.KeyEnter, 0, 0))
		clearTextField(t, pane)
		paste(pane, "tdd grill code-review")
		require.True(t, key(pane, xui.KeyEnter, 0, 0))

		emitted := savePendingStep(t, pane, store)
		require.Len(t, emitted, 1)
		assert.Equal(t, []string{"tdd", "grill", "code-review"}, emitted[0].Skills)
		assert.Equal(t, []string{"grill"}, emitted[0].DisabledSkills)
	})

	t.Run("dropped name takes its off mark along", func(t *testing.T) {
		store := skillStore()
		pane := newPane(store)
		openPendingStepDetail(t, pane)

		selectRow(t, pane, "⚙ 1 inject_skill · skills")
		require.True(t, key(pane, xui.KeyEnter, 0, 0))
		clearTextField(t, pane)
		paste(pane, "tdd")
		require.True(t, key(pane, xui.KeyEnter, 0, 0))

		emitted := savePendingStep(t, pane, store)
		require.Len(t, emitted, 1)
		assert.Equal(t, []string{"tdd"}, emitted[0].Skills)
		assert.Empty(t, emitted[0].DisabledSkills,
			"an off mark dies with the name that left the list")
	})
}
