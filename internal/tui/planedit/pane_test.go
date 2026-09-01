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
	created  []session.PlanV2
	err      error
	// interfere runs once, in place of the first commit: it lets a test move
	// the plan under an open modal the way the agent does.
	interfere func(*fakeStore, uint64) error
}

type appliedPatch struct {
	rev uint64
	ops []session.PlanPatchOp
}

func (s *fakeStore) Snapshot() session.Plan { return s.snapshot }

func (s *fakeStore) Create(_ context.Context, contract session.PlanV2) (session.Plan, error) {
	s.created = append(s.created, contract)
	if s.err != nil {
		return session.Plan{}, s.err
	}
	return s.snapshot, nil
}

func (s *fakeStore) StepTypes() []session.StepType {
	return append([]session.StepType(nil), s.types...)
}

func (s *fakeStore) Apply(_ context.Context, rev uint64, ops []session.PlanPatchOp) (session.Plan, error) {
	s.applied = append(s.applied, appliedPatch{rev: rev, ops: append([]session.PlanPatchOp(nil), ops...)})
	if s.interfere != nil {
		race := s.interfere
		s.interfere = nil
		if err := race(s, rev); err != nil {
			return session.Plan{}, err
		}
	}
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
	// 84 keeps the panel single-pane; wider screens preview the selection in a
	// second column, which truncates the list rows this test reads.
	browse := renderText(t, pane, 84, 30)
	assert.Contains(t, browse, "1 ▸ edit wire-pane — wire the pane")
	assert.NotContains(t, browse, "Done when:", "step fields stay out of the compact browser")

	key(pane, xui.KeyEnd, 0, 0) // Settings section tail.
	for range 4 {               // Back over the model rows to the pending step.
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0)
	assert.True(t, pane.State().Detail)
	detail := renderText(t, pane, 84, 30)
	assert.Contains(t, detail, "Step 2/2 test-pane", "the title names the open step")
	assert.Contains(t, detail, "ID: test-pane")
	assert.Contains(t, detail, "Type: run")
	assert.Contains(t, detail, "Status: pending")
	assert.Contains(t, detail, "Done when: focused tests pass")
	assert.Contains(t, detail, "Delete pending step…")
	assert.NotContains(t, detail, "Move step up", "reordering lives on Alt+↑↓, not on rows")
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

	// The chooser returns with the type row still selected; one step down is
	// content, then the three required prose fields in order.
	down(t, pane, 1)
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

// TestPaneCreatesTheFirstStepOfAnEmptyPlan pins the case the editor used to
// refuse outright: a plan with no steps yet is authored from the pane, and the
// insert carries no anchor because there is nothing to anchor to.
func TestPaneCreatesTheFirstStepOfAnEmptyPlan(t *testing.T) {
	plan := fixturePlan()
	plan.Items = nil
	store := &fakeStore{snapshot: plan}
	pane := newPane(store)
	assert.Contains(t, renderText(t, pane, 100, 30), "Add step", "an empty plan still offers the affordance")

	key(pane, xui.KeyEnd, 0, 0)
	for range 3 { // Back over the model rows to "Add step".
		key(pane, xui.KeyUp, 0, 0)
	}
	key(pane, xui.KeyEnter, 0, 0) // Add step opens detail, ID selected.

	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "first-step")
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Type chooser.
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Choose configured edit.
	key(pane, xui.KeyEnter, 0, 0)

	// The chooser returns with the type row still selected; one step down is
	// content, then the three required prose fields in order.
	down(t, pane, 1)
	for i, value := range []string{"start the plan", "the plan needs a first step", "the step exists"} {
		key(pane, xui.KeyEnter, 0, 0)
		paste(pane, value)
		key(pane, xui.KeyEnter, 0, 0)
		if i < 2 {
			key(pane, xui.KeyDown, 0, 0)
		}
	}
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	assert.Empty(t, pane.State().Error)
	require.Len(t, store.applied, 1)
	inserts := findOps(store.applied[0].ops, session.PlanPatchInsertStep)
	require.Len(t, inserts, 1)
	require.NotNil(t, inserts[0].Step)
	assert.Equal(t, "first-step", inserts[0].Step.ID)
	assert.Equal(t, session.StepEdit, inserts[0].Step.Type)
	assert.Empty(t, inserts[0].Before)
	assert.Empty(t, inserts[0].After)
	assert.Empty(t, findOps(store.applied[0].ops, session.PlanPatchReorderSteps), "one insert needs no reorder")
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
	key(pane, xui.KeyDelete, 0, 0)
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

// TestPaneDeletesOnlyOnTheAdvertisedKey pins deletion to the key the footer
// promises. Backspace deleted too, unannounced, in a list where it reads as
// "go back" — so it now says what does work instead of silently destroying a
// row.
func TestPaneDeletesOnlyOnTheAdvertisedKey(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	key(pane, xui.KeyEnd, 0, 0)
	for range 4 { // Back over the model rows to the pending step.
		key(pane, xui.KeyUp, 0, 0)
	}

	key(pane, xui.KeyBackspace, 0, 0)
	assert.False(t, pane.State().Confirming, "backspace never deletes")
	assert.Contains(t, pane.State().Error, "Del", "an unbound key names the key that works")

	key(pane, xui.KeyDelete, 0, 0)
	assert.True(t, pane.State().Confirming, "Del is the advertised delete")
}

// TestPaneReordersWithAltArrowsOnTheList: Alt+↑↓ moves the selected step
// inside the list it reorders, the selection rides along, and the applied
// patch carries one complete permutation.
func TestPaneReordersWithAltArrowsOnTheList(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	selectRow(t, pane, "wire the pane")
	require.True(t, key(pane, xui.KeyDown, 0, xui.ModAlt))
	assert.True(t, selectedRowContains(t, pane, "wire the pane"), "the selection follows the moved step")
	require.True(t, pane.State().Dirty)
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	reorders := findOps(store.applied[0].ops, session.PlanPatchReorderSteps)
	require.Len(t, reorders, 1, "one complete permutation represents the move")
	assert.Equal(t, []string{"test-pane", "wire-pane"}, reorders[0].IDs)
}

// TestPaneReordersFromStepDetails: the same chord works while the step's
// details are open — the title tracks the new position, so the move is
// visible there too.
func TestPaneReordersFromStepDetails(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	selectRow(t, pane, "test the pane")
	key(pane, xui.KeyEnter, 0, 0)
	require.True(t, pane.State().Detail)

	require.True(t, key(pane, xui.KeyUp, 0, xui.ModAlt))
	assert.Contains(t, renderText(t, pane, 100, 30), "Step 1/2 test-pane", "the title tracks the new position")
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	require.Len(t, store.applied, 1)
	reorders := findOps(store.applied[0].ops, session.PlanPatchReorderSteps)
	require.Len(t, reorders, 1)
	assert.Equal(t, []string{"test-pane", "wire-pane"}, reorders[0].IDs)
}

// TestPaneAltArrowsWantAStepRow: on a row that is not a step there is
// nothing to move; the chord says what it needs instead of doing nothing.
func TestPaneAltArrowsWantAStepRow(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	require.True(t, key(pane, xui.KeyDown, 0, xui.ModAlt)) // The goal row.
	assert.Equal(t, "alt+↑↓ moves a step — select a step row first", pane.State().Error)
	assert.False(t, pane.State().Dirty)
}

// TestPaneShiftArrowsNeverReorder: Shift+↑↓ extends a selection everywhere
// else in the TUI, so pressing it here must not mutate the plan — it names
// the chord that moves a step instead.
func TestPaneShiftArrowsNeverReorder(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	selectRow(t, pane, "wire the pane")
	require.True(t, key(pane, xui.KeyDown, 0, xui.ModShift))
	assert.Contains(t, pane.State().Error, "alt+↑↓", "the notice names the chord that works")
	assert.False(t, pane.State().Dirty, "a selection chord never mutates the plan")
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

// TestPaneTextPopupSavesTheFieldOnly pins where the durable write lives: Ctrl+S
// inside a field popup saves that field and nothing more, so the plan is only
// ever committed from the step list, which is the level that advertises it.
func TestPaneTextPopupSavesTheFieldOnly(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal is initially selected.
	paste(pane, " again")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	assert.False(t, pane.State().Editing, "the field is saved and the popup closed")
	assert.True(t, pane.State().Dirty)
	assert.Empty(t, store.applied, "a field editor never writes the plan")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1, "the step list is where Ctrl+S applies")
	assert.Equal(t, "ship the plan editor again", store.snapshot.Goal)
}

func TestPaneStaleRevisionKeepsTheDraftOpen(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan(), err: errors.New("session: plan revision 4 is stale (current 6)")}
	pane := newPane(store)
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, " changed")
	key(pane, xui.KeyRune, 's', xui.ModCtrl) // Saves the field, back to the list.
	key(pane, xui.KeyRune, 's', xui.ModCtrl) // Applies.

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
	plan.Schema = session.PlanSchemaLegacy
	pane := newPane(&fakeStore{snapshot: plan})
	require.True(t, pane.State().Readonly)
	key(pane, xui.KeyEnter, 0, 0)
	assert.False(t, pane.State().Editing)
	assert.Contains(t, pane.State().Error, "v2")
}

// TestPaneCreatesTheFirstPlanOfAPlanlessSession pins the reported bug: a
// session with no plan at all (schema zero, nothing durable) used to read
// as a legacy plan and locked read-only. It is a fresh editable draft, and
// saving it creates the v2 contract instead of patching nothing.
func TestPaneCreatesTheFirstPlanOfAPlanlessSession(t *testing.T) {
	store := &fakeStore{snapshot: session.Plan{}}
	pane := newPane(store)
	require.False(t, pane.State().Readonly, "an absent plan is a fresh draft, not a legacy one")

	key(pane, xui.KeyEnter, 0, 0) // Goal is initially selected.
	paste(pane, "author the first plan")
	key(pane, xui.KeyEnter, 0, 0)
	down(t, pane, 1) // Approach.
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "draft it from the pane")
	key(pane, xui.KeyEnter, 0, 0)
	selectRow(t, pane, "+ Add success criterion")
	key(pane, xui.KeyEnter, 0, 0)
	paste(pane, "the session has a plan")
	key(pane, xui.KeyEnter, 0, 0)
	selectRow(t, pane, "+ Add step")
	key(pane, xui.KeyEnter, 0, 0)

	key(pane, xui.KeyEnter, 0, 0) // ID.
	paste(pane, "first-step")
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Type chooser.
	key(pane, xui.KeyEnter, 0, 0)
	key(pane, xui.KeyDown, 0, 0) // Choose configured edit.
	key(pane, xui.KeyEnter, 0, 0)

	// The chooser returns with the type row still selected; one step down is
	// content, then why and done-when in order.
	down(t, pane, 1)
	for i, value := range []string{"start the plan", "the plan needs a first step", "the step exists"} {
		key(pane, xui.KeyEnter, 0, 0)
		paste(pane, value)
		key(pane, xui.KeyEnter, 0, 0)
		if i < 2 {
			key(pane, xui.KeyDown, 0, 0)
		}
	}
	key(pane, xui.KeyRune, 's', xui.ModCtrl)

	assert.Empty(t, pane.State().Error)
	require.Len(t, store.created, 1)
	contract := store.created[0]
	assert.Equal(t, "author the first plan", contract.Goal)
	assert.Equal(t, "draft it from the pane", contract.Approach)
	assert.Equal(t, []string{"the session has a plan"}, contract.SuccessCriteria)
	require.Len(t, contract.Items, 1)
	assert.Equal(t, "first-step", contract.Items[0].ID)
	assert.Equal(t, session.StepEdit, contract.Items[0].Type)
	assert.Equal(t, "start the plan", contract.Items[0].Content)
	assert.Empty(t, store.applied, "the first plan is created, not patched")
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
	for range 4 { // Back, model, add action, delete, then the JIT toggle.
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

// TestPaneChoiceListRefusesDeleteKeys: a choice list picks a value; Del and
// Backspace delete nothing there and say what the list answers instead.
func TestPaneChoiceListRefusesDeleteKeys(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan(), models: []string{"plan-a", "plan-b"}}
	pane := newPane(store)
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyDelete, 0, 0))
	assert.Equal(t, "this list only picks — Enter chooses, Esc goes back", pane.State().Error)
	assert.False(t, pane.State().Confirming, "nothing was armed for deletion")

	require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	assert.Equal(t, "this list only picks — Enter chooses, Esc goes back", pane.State().Error)
	assert.False(t, pane.State().Dirty)
}

// TestPaneModelPickerOpensOnCurrentPin: a choice list opens with the cursor
// on the value the row already has, not at the top.
func TestPaneModelPickerOpensOnCurrentPin(t *testing.T) {
	plan := fixturePlan()
	plan.ModelsByType = map[session.StepType]string{session.StepExplore: "plan-b"}
	store := &fakeStore{snapshot: plan, models: []string{"plan-a", "plan-b"}}
	pane := newPane(store)
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, selectedRowContains(t, pane, "plan-b"), "the cursor starts on the pinned model")
}

// TestPaneSpeaksVimMotions: j/k step and gg/G jump between the edges — the
// same dialect the context browser speaks.
func TestPaneSpeaksVimMotions(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	start := pane.State().Selected
	require.True(t, key(pane, xui.KeyRune, 'j', 0))
	assert.Greater(t, pane.State().Selected, start)
	require.True(t, key(pane, xui.KeyRune, 'k', 0))
	assert.Equal(t, start, pane.State().Selected)

	require.True(t, key(pane, xui.KeyRune, 'G', xui.ModShift))
	assert.Greater(t, pane.State().Selected, start)
	key(pane, xui.KeyRune, 'g', 0)
	key(pane, xui.KeyRune, 'g', 0)
	assert.Equal(t, start, pane.State().Selected)
}

// TestPaneSpeaksCountedMotions: a count prefixes a motion, exactly as in
// every other list — 3j lands where three single j's land.
func TestPaneSpeaksCountedMotions(t *testing.T) {
	counted := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(counted, xui.KeyRune, '3', 0))
	require.True(t, key(counted, xui.KeyRune, 'j', 0))

	stepped := newPane(&fakeStore{snapshot: fixturePlan()})
	for range 3 {
		require.True(t, key(stepped, xui.KeyRune, 'j', 0))
	}
	assert.Equal(t, stepped.State().Selected, counted.State().Selected)
	assert.Positive(t, counted.State().Selected)
}

// TestPaneMotionWithdrawsAnArmedConfirm: acting elsewhere withdraws the
// question, so a stale y can never delete what the cursor left behind.
func TestPaneMotionWithdrawsAnArmedConfirm(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	selectRow(t, pane, "test the pane")
	key(pane, xui.KeyDelete, 0, 0)
	require.True(t, pane.State().Confirming)

	require.True(t, key(pane, xui.KeyRune, 'k', 0))
	assert.False(t, pane.State().Confirming, "moving withdraws the question")
	require.True(t, key(pane, xui.KeyRune, 'y', 0))
	assert.False(t, pane.State().Dirty, "the stale y deletes nothing")
	// 84: single pane, so the step row is not truncated by the master column.
	assert.Contains(t, renderText(t, pane, 84, 30), "test the pane")
}

// TestPaneDeleteConfirmationNamesItsTarget: the y/n question names the step
// id, or quotes the directive, that it is about to drop.
func TestPaneDeleteConfirmationNamesItsTarget(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := newPane(store)
	selectRow(t, pane, "test the pane")
	key(pane, xui.KeyDelete, 0, 0)
	require.True(t, pane.State().Confirming)
	assert.Contains(t, renderText(t, pane, 100, 30), `Delete pending step "test-pane"? (y/n)`)
	key(pane, xui.KeyRune, 'n', 0)

	selectRow(t, pane, "patches apply atomically")
	key(pane, xui.KeyDelete, 0, 0)
	require.True(t, pane.State().Confirming)
	assert.Contains(t, renderText(t, pane, 100, 30),
		`Delete success criterion 1, "patches apply atomically"? (y/n)`)
}
