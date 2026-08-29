package planedit_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

func (s *fakeStore) Models() []string {
	return append([]string(nil), s.models...)
}

func actionFixturePlan() session.Plan {
	plan := fixturePlan()
	plan.ModelsByType = map[session.StepType]string{session.StepExplore: "plan-a"}
	plan.Items[0].Model = "plan-b"
	plan.Items[0].Actions = []session.PlanAction{{
		Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
		Runs: []session.PlanActionRun{{Status: session.PlanActionRunOK}},
	}}
	plan.Items[1].Actions = []session.PlanAction{{
		Event: session.PlanActionOnStepEnd, Type: session.PlanActionInjectSkill, Skills: []string{"tdd"},
	}}
	return plan
}

func actionStore() *fakeStore {
	return &fakeStore{
		snapshot: actionFixturePlan(),
		types:    []session.StepType{session.StepExplore, session.StepEdit, session.StepRun},
		models:   []string{"plan-a", "plan-b"},
	}
}

// selectRow moves the selection down until the row containing want is
// selected, so tests survive row insertions around them.
func selectRow(t *testing.T, pane *planedit.Pane, want string) {
	t.Helper()
	for range 80 {
		if selectedRowContains(t, pane, want) {
			return
		}
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	t.Fatalf("row %q not found in the rendered pane", want)
}

func selectedRowContains(t *testing.T, pane *planedit.Pane, want string) bool {
	t.Helper()
	for line := range strings.SplitSeq(renderText(t, pane, 100, 40), "\n") {
		// "›" is the selection marker and appears on exactly one row.
		if strings.Contains(line, "›") && strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func openPendingStepDetail(t *testing.T, pane *planedit.Pane) {
	t.Helper()
	// End lands on the last settings row; four Ups walk back over the model
	// section to the pending step.
	require.True(t, key(pane, xui.KeyEnd, 0, 0))
	for range 4 {
		require.True(t, key(pane, xui.KeyUp, 0, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
}

func TestDetailShowsModelOverrideAndActionRows(t *testing.T) {
	pane := newPane(actionStore())
	openPendingStepDetail(t, pane)

	text := renderText(t, pane, 100, 40)
	assert.Contains(t, text, "Model: (type default)", "an unpinned step says what it follows")
	assert.Contains(t, text, "⚙ 1 inject_skill@step_end", "authored actions are listed with their event")
}

func TestStepModelOverridePickerPatchesImmediately(t *testing.T) {
	store := actionStore()
	pane := newPane(store)
	openPendingStepDetail(t, pane)

	selectRow(t, pane, "Model:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	picker := renderText(t, pane, 100, 40)
	assert.Contains(t, picker, "(type default)")
	assert.Contains(t, picker, "plan-a")
	assert.Contains(t, picker, "plan-b")

	selectRow(t, pane, "plan-b")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "Model: plan-b")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	assert.Equal(t, "test-pane", updates[0].ID)
	assert.True(t, updates[0].Model.Set)
	assert.Equal(t, "plan-b", updates[0].Model.Value)
	assert.False(t, updates[0].Actions.Set, "untouched actions emit nothing")
	assert.Empty(t, findOps(store.applied[0].ops, session.PlanPatchSetPlanFields))
}

func TestStepActionsAddCycleSwitchAndSkills(t *testing.T) {
	store := actionStore()
	pane := newPane(store)
	openPendingStepDetail(t, pane)

	selectRow(t, pane, "+ Add action")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "⚙ 2 compact@step_start")

	// Cycle the new action's event, then switch it to inject_skill.
	selectRow(t, pane, "⚙ 2 compact@")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "⚙ 2 compact@step_end")

	selectRow(t, pane, "⚙ 2 compact · type")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "⚙ 2 inject_skill@step_end")

	selectRow(t, pane, "⚙ 2 inject_skill · skills")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	paste(pane, "tdd grill")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "skills: tdd, grill")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	require.True(t, updates[0].Actions.Set)
	emitted := updates[0].Actions.Value
	require.Len(t, emitted, 2)
	assert.Equal(t, session.PlanActionInjectSkill, emitted[0].Type)
	assert.Equal(t, []string{"tdd"}, emitted[0].Skills)
	assert.Equal(t, session.PlanActionInjectSkill, emitted[1].Type)
	assert.Equal(t, session.PlanActionOnStepEnd, emitted[1].Event)
	assert.Equal(t, []string{"tdd", "grill"}, emitted[1].Skills)
	for _, action := range emitted {
		assert.Empty(t, action.Runs, "authored actions never carry run history")
	}
}

func TestStepActionRemove(t *testing.T) {
	store := actionStore()
	pane := newPane(store)
	openPendingStepDetail(t, pane)

	selectRow(t, pane, "⚙ 1 inject_skill · remove")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.NotContains(t, renderText(t, pane, 100, 40), "inject_skill@")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	require.True(t, updates[0].Actions.Set)
	assert.Empty(t, updates[0].Actions.Value)
}

func TestAuthoredActionsStripRuns(t *testing.T) {
	store := actionStore()
	pane := newPane(store)

	// Open the in-progress step whose action carries run history.
	require.True(t, key(pane, xui.KeyEnd, 0, 0))
	for range 80 {
		if selectedRowContains(t, pane, "wire the pane") {
			break
		}
		require.True(t, key(pane, xui.KeyUp, 0, 0))
	}
	require.True(t, selectedRowContains(t, pane, "wire the pane"))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	selectRow(t, pane, "⚙ 1 compact@step_start")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
	require.Len(t, updates, 1)
	require.True(t, updates[0].Actions.Set)
	require.Len(t, updates[0].Actions.Value, 1)
	assert.Empty(t, updates[0].Actions.Value[0].Runs, "run history never re-enters through the editor")
	assert.Equal(t, session.PlanActionOnStepEnd, updates[0].Actions.Value[0].Event)
}

func TestModelsByTypePickerSetsAndClears(t *testing.T) {
	store := actionStore()
	pane := newPane(store)

	text := renderText(t, pane, 100, 40)
	assert.Contains(t, text, "Step models", "the settings section exists in browse")
	assert.Contains(t, text, "explore: plan-a")

	// Clear the explore pin.
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "(type default)")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 100, 40), "explore: (type default)")

	// Pin run to plan-b.
	selectRow(t, pane, "run:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "plan-b")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	fields := findOps(store.applied[0].ops, session.PlanPatchSetPlanFields)
	require.Len(t, fields, 1)
	require.True(t, fields[0].ModelsByType.Set)
	assert.Equal(t, map[session.StepType]string{session.StepRun: "plan-b"}, fields[0].ModelsByType.Value)
	assert.Empty(t, findOps(store.applied[0].ops, session.PlanPatchUpdateStep),
		"the type map touches no steps")
}
