package settings_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

type fakeStore struct {
	snapshot      harnesssettings.Snapshot
	snapshotReads int
	applied       []harnesssettings.Draft
	err           error
}

func (s *fakeStore) Snapshot() harnesssettings.Snapshot {
	s.snapshotReads++
	return s.snapshot
}

func (s *fakeStore) Apply(_ context.Context, draft harnesssettings.Draft) (harnesssettings.Snapshot, error) {
	s.applied = append(s.applied, draft)
	if s.err != nil {
		return harnesssettings.Snapshot{}, s.err
	}
	s.snapshot.Token = "committed"
	s.snapshot.Plan = draft.Plan
	return s.snapshot, nil
}

func fixtureStore() *fakeStore {
	return &fakeStore{snapshot: harnesssettings.Snapshot{
		Token: "opened",
		Path:  "/home/test/.cozyphi/config.yaml",
		Plan:  plangate.DefaultDefaults(),
	}}
}

func key(p *settings.Pane, code xui.KeyCode, r rune, mods xui.Modifiers) bool {
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r, Mods: mods})
}

func TestPaneShowTabsAndEscapeDiscardsDraft(t *testing.T) {
	store := fixtureStore()
	closed := 0
	pane := settings.New(components.DefaultTheme(), store, func() { closed++ })

	pane.Show()
	require.True(t, pane.Visible())
	assert.Equal(t, settings.TabPlanDefaults, pane.State().Tab)
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Equal(t, settings.TabGeneral, pane.State().Tab)
	require.True(t, key(pane, xui.KeyTab, 0, xui.ModShift))
	assert.Equal(t, settings.TabPlanDefaults, pane.State().Tab)

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.Visible())
	assert.Empty(t, store.applied, "Esc closes without persisting the draft")
	assert.Equal(t, 1, closed)
}

func TestPaneCtrlSAppliesWholeDraftAndClosesOnSuccess(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	var applied harnesssettings.Snapshot
	pane.SetOnApplied(func(snapshot harnesssettings.Snapshot) { applied = snapshot })
	pane.Show()

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, "opened", store.applied[0].BaseToken)
	assert.Equal(t, "committed", applied.Token)
	assert.False(t, pane.Visible())
}

func TestPaneApplyErrorKeepsDraftOpen(t *testing.T) {
	store := fixtureStore()
	store.err = errors.New("disk full")
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	assert.True(t, pane.Visible())
	assert.Contains(t, pane.State().Error, "disk full")
}

func TestPaneKeepsIndependentTabScrollAndShowsOverflow(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	pane.Draw(components.DrawContext{Max: components.Size{Width: 64, Height: 9}, Method: xui.WidthUnicode})
	require.True(t, pane.State().Overflow)

	require.True(t, pane.HandleEvent(&components.EventContext{}, xui.MouseEvent{
		Button: xui.MouseWheelDown,
		Wheel:  2,
	}))
	planScroll := pane.State().Scroll
	require.Positive(t, planScroll)

	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Zero(t, pane.State().Scroll)
	pane.Draw(components.DrawContext{Max: components.Size{Width: 64, Height: 7}, Method: xui.WidthUnicode})
	require.True(t, pane.HandleEvent(&components.EventContext{}, xui.MouseEvent{
		Button: xui.MouseWheelDown,
		Wheel:  1,
	}))
	generalScroll := pane.State().Scroll
	require.Positive(t, generalScroll)

	require.True(t, key(pane, xui.KeyTab, 0, xui.ModShift))
	assert.Equal(t, planScroll, pane.State().Scroll)
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Equal(t, generalScroll, pane.State().Scroll)
}

func TestPaneMouseSelectsRenderedTab(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	surface := pane.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 20}, Method: xui.WidthUnicode})
	x, y := findText(t, surface, "General")

	require.True(t, pane.HandleEvent(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress,
		Button: xui.MouseLeft,
		X:      x,
		Y:      y,
	}))
	assert.Equal(t, settings.TabGeneral, pane.State().Tab)
}

func TestPaneAddsAndRenamesNewTypeWithoutCreatingMigrationIntent(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	clickRow(t, pane, "Add type")
	typeName(t, pane, "Bad")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, pane.State().Error, "invalid")
	assert.Contains(t, drawText(pane), "Add type: Bad", "invalid input stays open")
	for range len("Bad") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}

	typeName(t, pane, "edit")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, pane.State().Error, "duplicate")
	assert.Contains(t, drawText(pane), "Add type: edit", "duplicate input stays open")
	for range len("edit") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	typeName(t, pane, "audit")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, pane.State().Dirty)

	clickRow(t, pane, "Rename type audit")
	for range len("audit") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	typeName(t, pane, "review")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	clickRow(t, pane, "Rename type review")
	for range len("review") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	typeName(t, pane, "final")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	got := store.applied[0]
	assert.Equal(t, session.StepType("final"), got.Plan.Types[len(got.Plan.Types)-1].Name)
	assert.Empty(t, got.TypeRenames, "renaming a type created in this draft does not migrate the current plan")
}

func TestPaneChainsRenamesForTypeThatExistedWhenOpened(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	clickRow(t, pane, "Rename type explore")
	for range len("explore") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	typeName(t, pane, "inspect")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	clickRow(t, pane, "Rename type inspect")
	for range len("inspect") {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	typeName(t, pane, "research")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	assert.Equal(t, session.StepType("research"), store.applied[0].TypeRenames[session.StepExplore])
	assert.NotContains(t, store.applied[0].TypeRenames, session.StepType("inspect"))
}

func TestPaneEscapeCancelsNameEntryWithoutClosingModal(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	clickRow(t, pane, "Add type")
	typeName(t, pane, "scratch")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.True(t, pane.Visible())
	assert.False(t, pane.State().Dirty)
	assert.NotContains(t, drawText(pane), "Add type: scratch")
}

func TestPaneTypeModelPickAndApply(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.SetModelNames([]string{"plan-a", "plan-b"})
	pane.Show()

	selectRow(t, pane, "Model: (session default)")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, drawText(pane), "plan-a", "Enter expands the inline model list")

	selectRow(t, pane, "plan-a")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, drawText(pane), "Model: plan-a")
	require.True(t, pane.State().Dirty)

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, "plan-a", store.applied[0].Plan.Types[0].Model,
		"Ctrl+S persists the type's model pin in the plan defaults")
}

func TestPaneBlocksDeletingTypeUsedByCurrentPlan(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.SetTypeInUse(func(typ session.StepType) bool { return typ == session.StepExplore })
	pane.Show()

	selectRow(t, pane, "Delete type explore")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	assert.False(t, pane.State().Dirty)
	assert.Contains(t, pane.State().Error, "current plan")
	assert.Contains(t, drawText(pane), "Type: explore")
}

func TestPaneDeletesUnusedTypeAndAppliesWholeDraft(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.SetTypeInUse(func(session.StepType) bool { return false })
	pane.Show()

	selectRow(t, pane, "Delete type integrate")
	require.True(t, key(pane, xui.KeyRune, ' ', 0))
	assert.True(t, pane.State().Dirty)
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	for _, typ := range store.applied[0].Plan.Types {
		assert.NotEqual(t, session.StepIntegrate, typ.Name)
	}
}

func TestPanePermissionTogglesCascadeAndOutsidePlanRemovesAssignment(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	selectRow(t, pane, "bash · for explore")
	require.True(t, key(pane, xui.KeyRune, ' ', 0), "Space enables bash at the least-capable type")
	selectRow(t, pane, "read · for edit")
	require.True(t, key(pane, xui.KeyEnter, 0, 0), "Enter disables read at edit and every lower type")
	clickRow(t, pane, "write · allowed outside plan")
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	got := store.applied[0].Plan
	assert.Equal(t, 0, assignmentRank(got, "bash"))
	assert.Equal(t, 2, assignmentRank(got, "read"))
	assert.Equal(t, -1, assignmentRank(got, "write"))
	assert.Contains(t, got.AdditionalExemptions, "write")
	for _, tool := range []string{"bash", "read", "write"} {
		assert.LessOrEqual(t, assignmentCount(got, tool), 1, tool)
	}
}

func TestPaneResetRestoresBuiltInsAndMouseActivatesAction(t *testing.T) {
	store := fixtureStore()
	store.snapshot.Plan.Types = store.snapshot.Plan.Types[:2]
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	clickRow(t, pane, "Reset built-in defaults")
	assert.True(t, pane.State().Dirty)
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	assert.Equal(t, plangate.DefaultDefaults(), store.applied[0].Plan)
}

func TestPanePlanTabListsAvailableSkills(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.SetSkills([]string{"tdd", "code-review"})
	pane.Show()

	text := drawText(pane)
	assert.Contains(t, text, "skills: tdd, code-review", "the plan tab enumerates available skills")
	assert.NotContains(t, text, "inject_skill", "the action type stays out of the vocabulary")
}

func TestPaneShowsKnownToolAvailabilityAndLockedMandatoryExemptions(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.SetAvailableTools([]string{"read", "plan"})
	pane.Show()

	text := drawText(pane)
	assert.Contains(t, text, "read · for explore · available")
	assert.Contains(t, text, "grep · for explore · unavailable")

	selectRow(t, pane, "plan · always allowed (locked)")
	assert.Contains(t, drawText(pane), "plan · always allowed (locked) · available")
	require.True(t, key(pane, xui.KeyRune, ' ', 0))
	assert.False(t, pane.State().Dirty)
}

func TestPaneShowsMandatoryToolsWhenNoStepTypesExist(t *testing.T) {
	store := fixtureStore()
	store.snapshot.Plan.Types = nil
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	text := drawText(pane)
	assert.Contains(t, text, "plan · always allowed (locked)")
	assert.Contains(t, text, "context · always allowed (locked)")
}

func TestPaneDrawHandlesTinyBoundsWithoutRereadingSnapshot(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.Equal(t, 1, store.snapshotReads)

	assert.NotPanics(t, func() {
		pane.Draw(components.DrawContext{Max: components.Size{Width: 1, Height: 1}, Method: xui.WidthUnicode})
		pane.Draw(components.DrawContext{Max: components.Size{Width: 2, Height: 2}, Method: xui.WidthUnicode})
	})
	assert.Equal(t, 1, store.snapshotReads)
}

func drawText(pane *settings.Pane) string {
	return components.SurfaceText(pane.Draw(components.DrawContext{
		Max: components.Size{Width: 110, Height: 30}, Method: xui.WidthUnicode,
	}))
}

func selectRow(t *testing.T, pane *settings.Pane, label string) {
	t.Helper()
	for range 300 {
		for line := range strings.SplitSeq(drawText(pane), "\n") {
			if strings.Contains(line, "› ") && strings.Contains(line, label) {
				return
			}
		}
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	t.Fatalf("row %q was not selectable", label)
}

func clickRow(t *testing.T, pane *settings.Pane, label string) {
	t.Helper()
	selectRow(t, pane, label)
	surface := pane.Draw(components.DrawContext{
		Max: components.Size{Width: 110, Height: 30}, Method: xui.WidthUnicode,
	})
	x, y := findText(t, surface, label)
	require.True(t, pane.HandleEvent(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress,
		Button: xui.MouseLeft,
		X:      x,
		Y:      y,
	}))
}

func typeName(t *testing.T, pane *settings.Pane, name string) {
	t.Helper()
	for _, r := range name {
		require.True(t, key(pane, xui.KeyRune, r, 0))
	}
}

func assignmentRank(defaults plangate.Defaults, tool string) int {
	for i, typ := range defaults.Types {
		if slices.Contains(typ.Tools, tool) {
			return i
		}
	}
	return -1
}

func assignmentCount(defaults plangate.Defaults, tool string) int {
	count := 0
	for _, typ := range defaults.Types {
		if slices.Contains(typ.Tools, tool) {
			count++
		}
	}
	return count
}

func findText(t *testing.T, surface components.Surface, needle string) (int, int) {
	t.Helper()
	want := []rune(needle)
	for y := 0; y < surface.Size.Height; y++ {
		for x := 0; x+len(want) <= surface.Size.Width; x++ {
			var got strings.Builder
			for i := range len(want) {
				got.WriteString(surface.Buffer[y*surface.Size.Width+x+i].Char)
			}
			if got.String() == needle {
				return x, y
			}
		}
	}
	t.Fatalf("%q not rendered", needle)
	return 0, 0
}
