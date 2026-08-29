package planedit_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

type fakeStore struct {
	snapshot session.Plan
	types    []session.StepType
	models   []string
	applied  []appliedPatch
	err      error
}

type appliedPatch struct {
	rev uint64
	ops []session.PlanPatchOp
}

func (s *fakeStore) Snapshot() session.Plan { return s.snapshot }

func (s *fakeStore) StepTypes() []session.StepType {
	return append([]session.StepType(nil), s.types...)
}

func (s *fakeStore) Apply(_ context.Context, rev uint64, ops []session.PlanPatchOp) (session.Plan, error) {
	s.applied = append(s.applied, appliedPatch{rev: rev, ops: append([]session.PlanPatchOp(nil), ops...)})
	if s.err != nil {
		return session.Plan{}, s.err
	}
	for _, op := range ops {
		if op.Op == session.PlanPatchSetPlanFields {
			if op.Goal.Set {
				s.snapshot.Goal = op.Goal.Value
			}
			if op.Approach.Set {
				s.snapshot.Approach = op.Approach.Value
			}
		}
		if op.Op == session.PlanPatchReplaceContext && op.WorkingContext.Set {
			s.snapshot.WorkingContext = op.WorkingContext.Value
		}
	}
	return s.snapshot, nil
}

func fixturePlan() session.Plan {
	return session.Plan{
		Revision: 4,
		Approved: true,
		Schema:   session.PlanSchemaV2,
		Goal:     "ship the plan editor",
		Approach: "settings browser behind a Store seam",
		SuccessCriteria: []string{
			"patches apply atomically",
			"stale revisions remain visible",
		},
		Constraints:    []string{"no new dependency", "keep the shell thin"},
		WorkingContext: "planedit owns the draft",
		Items: []session.PlanItem{
			{
				ID: "wire-pane", Content: "wire the pane", Type: session.StepEdit,
				Status: session.PlanInProgress, Why: "users need it", DoneWhen: "the modal opens",
			},
			{
				ID: "test-pane", Content: "test the pane", Type: session.StepRun,
				Status: session.PlanPending, Why: "regressions happen", DoneWhen: "focused tests pass",
			},
		},
	}
}

func newPane(store *fakeStore) *planedit.Pane {
	if store.types == nil {
		store.types = []session.StepType{session.StepExplore, session.StepEdit, session.StepRun}
	}
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()
	return pane
}

func key(p *planedit.Pane, code xui.KeyCode, r rune, mods xui.Modifiers) bool {
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r, Mods: mods})
}

func down(t *testing.T, pane *planedit.Pane, count int) {
	t.Helper()
	for range count {
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
}

func paste(p *planedit.Pane, value string) *components.EventContext {
	ctx := &components.EventContext{}
	p.HandleEvent(ctx, xui.PasteEvent{Text: value})
	return ctx
}

func renderText(t *testing.T, pane *planedit.Pane, width, height int) string {
	t.Helper()
	surface := pane.Draw(components.DrawContext{
		Max: components.Size{Width: width, Height: height}, Method: xui.WidthUnicode,
	})
	screen := xui.NewScreen(width, height)
	window := xui.NewWindow(screen)
	window.Clear()
	surface.Render(window)
	var text strings.Builder
	for y := range height {
		for x := range width {
			text.WriteString(screen.GetCell(x, y).Char)
		}
		text.WriteByte('\n')
	}
	return text.String()
}

func findOps(ops []session.PlanPatchOp, name string) []session.PlanPatchOp {
	var found []session.PlanPatchOp
	for _, op := range ops {
		if op.Op == name {
			found = append(found, op)
		}
	}
	return found
}

func TestPaneTextPopupSupportsCursorInsertionAndIsolatesPaste(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)

	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal is initially selected.
	require.True(t, pane.State().Editing)
	require.True(t, key(pane, xui.KeyLeft, 0, 0))
	require.True(t, key(pane, xui.KeyLeft, 0, 0))
	ctx := paste(pane, "X")
	assert.True(t, ctx.Consume)
	assert.Contains(t, renderText(t, pane, 80, 24), "editXor", "paste inserts at the TextField cursor")
	assert.Contains(t, renderText(t, pane, 80, 24), "Edit goal")

	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Editing)
	assert.True(t, pane.State().Dirty)

	ctx = paste(pane, "must not reach composer")
	assert.True(t, ctx.Consume, "paste remains owned by the visible modal outside the popup")
	assert.NotContains(t, renderText(t, pane, 80, 24), "must not reach composer")
}

func TestPaneCompilesWorkingContextSeparately(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	down(t, pane, 2) // Context.
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	paste(pane, " updated")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	require.Len(t, store.applied[0].ops, 1)
	op := store.applied[0].ops[0]
	assert.Equal(t, session.PlanPatchReplaceContext, op.Op)
	assert.True(t, op.WorkingContext.Set)
	assert.False(t, op.Goal.Set)
	assert.False(t, op.Approach.Set)
}

func TestPaneCompilesDirectiveAddUpdateDelete(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)

	down(t, pane, 3) // First criterion.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " updated")
	key(pane, xui.KeyEnter, 0, 0)
	down(t, pane, 2) // Explicit add row.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "new criterion")
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyUp, 0, 0) // Previous base criterion.
	key(pane, xui.KeyDelete, 0, 0)
	assert.True(t, pane.State().Confirming)
	key(pane, xui.KeyRune, 'y', 0)
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	ops := store.applied[0].ops
	updates := findOps(ops, session.PlanPatchUpdateCriterion)
	require.Len(t, updates, 2)
	assert.Equal(t, "patches apply atomically", updates[0].From)
	assert.Equal(t, "patches apply atomically updated", updates[0].To)
	assert.Equal(t, "stale revisions remain visible", updates[1].From)
	assert.Equal(t, "new criterion", updates[1].To)
	assert.Empty(t, findOps(ops, session.PlanPatchAddCriterion))
	assert.Empty(t, findOps(ops, session.PlanPatchRemoveCriterion))
}

func TestPaneCompilesConstraintAddUpdateDelete(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)

	down(t, pane, 6) // First constraint.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " updated")
	key(pane, xui.KeyEnter, 0, 0)
	down(t, pane, 2) // Explicit add row.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "new constraint")
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyUp, 0, 0)
	key(pane, xui.KeyDelete, 0, 0)
	key(pane, xui.KeyRune, 'y', 0)
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	ops := store.applied[0].ops
	updates := findOps(ops, session.PlanPatchUpdateConstraint)
	require.Len(t, updates, 2)
	assert.Equal(t, "no new dependency", updates[0].From)
	assert.Equal(t, "no new dependency updated", updates[0].To)
	assert.Equal(t, "keep the shell thin", updates[1].From)
	assert.Equal(t, "new constraint", updates[1].To)
	assert.Empty(t, findOps(ops, session.PlanPatchAddConstraint))
	assert.Empty(t, findOps(ops, session.PlanPatchRemoveConstraint))
}

func TestPaneShowsCompactStepsThenDetailForm(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	browse := renderText(t, pane, 100, 30)
	assert.Contains(t, browse, "1 ▸ edit wire-pane — wire the pane")
	assert.NotContains(t, browse, "Done when:", "step fields stay out of the compact browser")

	key(pane, xui.KeyEnd, 0, 0) // Settings section tail.
	for range 4 {               // Back over the model rows to the pending step.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)
	assert.True(t, pane.State().Detail)
	detail := renderText(t, pane, 100, 30)
	assert.Contains(t, detail, "Step 2/2 test-pane", "the title names the open step")
	assert.Contains(t, detail, "ID: test-pane")
	assert.Contains(t, detail, "Type: run")
	assert.Contains(t, detail, "Status: pending")
	assert.Contains(t, detail, "Done when: focused tests pass")
	assert.Contains(t, detail, "Move step up")
}

func TestPaneAddsPendingStepWithConfiguredType(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	key(pane, xui.KeyEnd, 0, 0)
	for range 3 { // Back over the model rows to "Add step".
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0) // Add step opens detail, ID selected.

	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "new-step")
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Type chooser.
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Choose configured edit.
	key(pane, xui.KeyEnter, 0, 0)

	// Selection reset to ID. Move through type to content, then fill the three required prose fields.
	down(t, pane, 2)
	for i, value := range []string{"implement it", "the feature is required", "focused tests pass"} {
		key(pane, xui.KeyEnter, 0, 0)
		paste(pane, value)
		key(pane, xui.KeyEnter, 0, 0)
		if i < 2 {
			key(pane, xui.KeyDown, 0, 0)
		}
	}
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	inserts := findOps(store.applied[0].ops, session.PlanPatchInsertStep)
	require.Len(t, inserts, 1)
	require.NotNil(t, inserts[0].Step)
	assert.Equal(t, "new-step", inserts[0].Step.ID)
	assert.Equal(t, session.StepEdit, inserts[0].Step.Type)
	assert.Equal(t, session.PlanPending, inserts[0].Step.Status)
	assert.Equal(t, "implement it", inserts[0].Step.Content)
	require.Len(t, findOps(store.applied[0].ops, session.PlanPatchReorderSteps), 1)
}

func TestPaneRefusesNonPendingDeleteAndConfirmsPendingDelete(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	key(pane, xui.KeyEnd, 0, 0)
	for range 5 { // Back over the model rows and both steps.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyDelete, 0, 0)
	assert.False(t, pane.State().Confirming)
	assert.Contains(t, pane.State().Error, "only pending")

	key(pane, xui.KeyDown, 0, 0)
	key(pane, xui.KeyBackspace, 0, 0)
	assert.True(t, pane.State().Confirming)
	key(pane, xui.KeyRune, 'n', 0)
	assert.False(t, pane.State().Confirming)
	key(pane, xui.KeyDelete, 0, 0)
	key(pane, xui.KeyRune, 'y', 0)
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	removes := findOps(store.applied[0].ops, session.PlanPatchRemoveStep)
	require.Len(t, removes, 1)
	assert.Equal(t, "test-pane", removes[0].ID)
}

func TestPaneReordersThroughVisibleDetailAction(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	key(pane, xui.KeyEnd, 0, 0)
	for range 4 { // Back over the model rows to the pending step.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0) // Pending step detail.
	key(pane, xui.KeyEnd, 0, 0)   // Back.
	for range 5 {                 // Model, add action, delete, move down, then move up.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	reorders := findOps(store.applied[0].ops, session.PlanPatchReorderSteps)
	require.Len(t, reorders, 1, "one complete permutation represents the move")
	assert.Equal(t, []string{"test-pane", "wire-pane"}, reorders[0].IDs)
}

func TestPaneDirtyEscapeRequiresDiscardConfirmation(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " changed")
	key(pane, xui.KeyEnter, 0, 0)
	require.True(t, pane.State().Dirty)

	key(pane, xui.KeyEscape, 0, 0)
	assert.True(t, pane.State().Confirming)
	assert.True(t, pane.Visible())
	key(pane, xui.KeyRune, 'n', 0)
	assert.True(t, pane.Visible())
	key(pane, xui.KeyEscape, 0, 0)
	key(pane, xui.KeyRune, 'y', 0)
	assert.False(t, pane.Visible())
	assert.Empty(t, store.applied)
}

func TestPaneStaleRevisionKeepsPopupDraftOpen(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan(), err: errors.New("session: plan revision 4 is stale (current 6)")}
	pane := newPane(store)
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " changed")
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	assert.Equal(t, uint64(4), store.applied[0].rev)
	assert.True(t, pane.Visible())
	assert.True(t, pane.State().Dirty)
	assert.Contains(t, pane.State().Error, "stale")
}

func TestPaneResizeOverflowAndLongValuesRemainSafe(t *testing.T) {
	plan := fixturePlan()
	plan.Goal = strings.Repeat("wide value ", 40)
	for range 6 {
		plan.Constraints = append(plan.Constraints, "constraint "+strings.Repeat("x", 80))
	}
	pane := newPane(&fakeStore{snapshot: plan})

	assert.NotPanics(t, func() {
		pane.Draw(components.DrawContext{Max: components.Size{Width: 1, Height: 1}, Method: xui.WidthUnicode})
	})
	key(pane, xui.KeyEnter, 0, 0)
	assert.NotPanics(t, func() {
		pane.Draw(components.DrawContext{Max: components.Size{Width: 1, Height: 1}, Method: xui.WidthUnicode})
	})
	key(pane, xui.KeyEscape, 0, 0)
	pane.Draw(components.DrawContext{Max: components.Size{Width: 36, Height: 7}, Method: xui.WidthUnicode})
	assert.True(t, pane.State().Overflow)
	key(pane, xui.KeyEnd, 0, 0)
	assert.Positive(t, pane.State().Scroll)
}

func TestPaneIDLessV2PlanRequiresMigrationBeforeAnyEditing(t *testing.T) {
	plan := fixturePlan()
	plan.Items[0].ID = ""
	pane := newPane(&fakeStore{snapshot: plan})

	require.True(t, pane.State().Readonly)
	key(pane, xui.KeyEnter, 0, 0) // Ordinary goal editing is refused too.
	assert.False(t, pane.State().Editing)
	assert.Contains(t, pane.State().Error, "migration is required")
}

func TestPaneCleanEscapeClosesWithoutApplying(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	key(pane, xui.KeyEscape, 0, 0)
	assert.False(t, pane.Visible())
	assert.Empty(t, store.applied)
}

func TestPaneLegacyPlanIsReadonly(t *testing.T) {
	plan := fixturePlan()
	plan.Schema = 0
	pane := newPane(&fakeStore{snapshot: plan})
	require.True(t, pane.State().Readonly)
	key(pane, xui.KeyEnter, 0, 0)
	assert.False(t, pane.State().Editing)
	assert.Contains(t, pane.State().Error, "v2")
}

func TestPaneRendersExistingJITPostureReadonly(t *testing.T) {
	plan := fixturePlan()
	plan.Items[1].JIT = true
	pane := newPane(&fakeStore{snapshot: plan})

	key(pane, xui.KeyEnd, 0, 0)
	for range 4 {
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)
	detail := renderText(t, pane, 100, 30)
	assert.Contains(t, detail, "Just-in-time: enabled (read-only after creation)")
	assert.NotContains(t, detail, "Toggle just-in-time posture")
}

func TestPaneOffersJITToggleForNewStep(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	key(pane, xui.KeyEnd, 0, 0)
	for range 3 { // Back over the model rows to "Add step".
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)

	before := renderText(t, pane, 100, 30)
	assert.Contains(t, before, "Toggle just-in-time posture (currently disabled)")
	key(pane, xui.KeyEnd, 0, 0)
	for range 6 { // Back, model, add action, delete, move down, move up, then the JIT toggle.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)

	assert.Contains(t, renderText(t, pane, 100, 30), "Toggle just-in-time posture (currently enabled)")
}

func TestPaneResizeFollowsSelectionIntoChangedViewport(t *testing.T) {
	plan := fixturePlan()
	for range 6 {
		plan.Constraints = append(plan.Constraints, "extra constraint")
	}
	pane := newPane(&fakeStore{snapshot: plan})
	pane.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 30}, Method: xui.WidthUnicode})
	down(t, pane, 8)
	selected := pane.State().Selected

	pane.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 7}, Method: xui.WidthUnicode})
	assert.Equal(t, selected, pane.State().Scroll, "one-row viewport follows the selected row after shrink")

	pane.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 30}, Method: xui.WidthUnicode})
	state := pane.State()
	assert.LessOrEqual(t, state.Scroll, state.Selected)
	assert.Less(t, state.Selected, state.Scroll+23, "selected row remains visible after regrow")
}
