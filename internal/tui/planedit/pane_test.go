package planedit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// fakeStore records every apply so tests can pin the revision guard and the
// patch payload without touching the durable session.
type fakeStore struct {
	snapshot session.Plan
	applied  []appliedPatch
	err      error
}

type appliedPatch struct {
	rev uint64
	ops []session.PlanPatchOp
}

func (s *fakeStore) Snapshot() session.Plan { return s.snapshot }

func (s *fakeStore) Apply(_ context.Context, rev uint64, ops []session.PlanPatchOp) (session.Plan, error) {
	s.applied = append(s.applied, appliedPatch{rev: rev, ops: ops})
	if s.err != nil {
		return session.Plan{}, s.err
	}
	for _, op := range ops {
		if op.Op == session.PlanPatchSetPlanFields && op.Goal.Set {
			s.snapshot.Goal = op.Goal.Value
		}
	}
	return s.snapshot, nil
}

func fixturePlan() session.Plan {
	return session.Plan{
		Revision: 4,
		Approved: true,
		Schema:   session.PlanSchemaV2,
		Goal:     "ship the inline plan editor",
		Approach: "settings.Pane pattern behind a Store seam",
		SuccessCriteria: []string{
			"apply round-trips through PatchPlan",
			"stale revisions surface an error",
		},
		Constraints:    []string{"no whole-file rewrite of the hashline editor"},
		WorkingContext: "internal/tui/planedit is the only editor",
		Items: []session.PlanItem{{
			ID:       "wire-pane",
			Content:  "wire the pane into the shell",
			Type:     session.StepEdit,
			Status:   session.PlanInProgress,
			Why:      "a hidden pane is dead code",
			DoneWhen: "Ctrl+P opens the editor",
		}},
	}
}

func key(p *planedit.Pane, code xui.KeyCode, r rune, mods xui.Modifiers) bool {
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r, Mods: mods})
}

func TestPaneCleanCloseSkipsStore(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	closed := 0
	pane := planedit.New(components.DefaultTheme(), store, func() { closed++ })
	pane.Show()
	require.True(t, pane.Visible())
	assert.False(t, pane.State().Readonly)

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	assert.False(t, pane.Visible(), "a clean draft closes without a patch round trip")
	assert.Empty(t, store.applied)
	assert.Equal(t, 1, closed)
}

func TestPaneEditsGoalAndAppliesPatch(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()
	var applied session.Plan
	pane.SetOnApplied(func(plan session.Plan) { applied = plan })

	// Row order: plan heading, then goal as the first editable field.
	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, pane.State().Editing)
	require.True(t, key(pane, xui.KeyRune, ' ', 0))
	require.True(t, key(pane, xui.KeyRune, 'v', 0))
	require.True(t, key(pane, xui.KeyRune, '2', 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, pane.State().Dirty)

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, uint64(4), store.applied[0].rev, "apply guards the snapshot revision")
	require.Len(t, store.applied[0].ops, 1)
	op := store.applied[0].ops[0]
	assert.Equal(t, session.PlanPatchSetPlanFields, op.Op)
	require.True(t, op.Goal.Set)
	assert.Equal(t, "ship the inline plan editor v2", op.Goal.Value)
	assert.False(t, pane.Visible())
	assert.Equal(t, "ship the inline plan editor v2", applied.Goal)
}

func TestPaneStaleRevisionKeepsDraftOpen(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	store.err = errors.New("session: plan revision 4 is stale (current 6)")
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()

	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, '!', 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, pane.State().Dirty)

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1, "the stale write was attempted exactly once")
	assert.True(t, pane.Visible(), "a refused patch keeps the modal open")
	assert.True(t, pane.State().Dirty)
	assert.Contains(t, pane.State().Error, "stale")
}

func TestPaneLegacyPlanIsReadonly(t *testing.T) {
	plan := fixturePlan()
	plan.Schema = 0 // legacy files predate the schema marker
	store := &fakeStore{snapshot: plan}
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, pane.State().Readonly)

	require.True(t, key(pane, xui.KeyDown, 0, 0))
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Editing)
	assert.Contains(t, pane.State().Error, "v2")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	assert.Empty(t, store.applied, "a legacy plan never reaches the store")
	assert.True(t, pane.Visible())
}

func TestPaneStepRowRefusesEditAndRenderSmoke(t *testing.T) {
	store := &fakeStore{snapshot: fixturePlan()}
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()

	// Rows: plan heading, goal, approach, context, 2 criteria, 1 constraint,
	// steps heading, then the step summary row.
	for range 8 {
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.False(t, pane.State().Editing)
	assert.Contains(t, pane.State().Error, "lifecycle")

	// Render smoke: a short viewport must overflow, not panic.
	pane.Draw(components.DrawContext{Max: components.Size{Width: 64, Height: 6}, Method: xui.WidthUnicode})
	assert.True(t, pane.State().Overflow)
}
