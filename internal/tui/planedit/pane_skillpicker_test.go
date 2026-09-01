package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// pickerPane is skillStore plus a wired catalog: the pane where the skills
// row opens the picker instead of the free-text editor.
func pickerPane(t *testing.T, catalog ...string) (*planedit.Pane, *fakeStore) {
	t.Helper()
	store := skillStore()
	pane := newPane(store)
	pane.SetSkills(catalog)
	openPendingStepDetail(t, pane)
	selectRow(t, pane, "⚙ 1 inject_skill · skills")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	return pane, store
}

func TestSkillsRowOpensThePickerWhenACatalogExists(t *testing.T) {
	pane, _ := pickerPane(t, "code-review", "tdd", "grill")

	assert.False(t, pane.State().Editing, "a catalog replaces the free-text editor with the picker")
	text := renderText(t, pane, 84, 40)
	assert.Contains(t, text, "Choose step skills")
	assert.Contains(t, text, "[ ] code-review")
	assert.Contains(t, text, "[x] tdd")
	assert.Contains(t, text, "[x] grill")
	assert.Contains(t, text, "other… (type a name)", "manual entry stays reachable")
	assert.True(t, selectedRowContains(t, pane, "[x]"),
		"the cursor opens on the first checked name, not on the top")
}

func TestPickerTogglesAndSavesTheCheckedSet(t *testing.T) {
	pane, store := pickerPane(t, "code-review", "tdd", "grill")

	selectRow(t, pane, "code-review")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 84, 40), "[x] code-review",
		"the toggle lands and the picker stays open")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.True(t, pane.State().Detail, "Esc keeps the checked set and returns to the step")
	emitted := savePendingStep(t, pane, store)
	require.Len(t, emitted, 1)
	assert.Equal(t, []string{"tdd", "grill", "code-review"}, emitted[0].Skills)
	assert.Equal(t, []string{"grill"}, emitted[0].DisabledSkills,
		"an untouched off mark survives the picker round trip")
}

func TestPickerUnchecksASkill(t *testing.T) {
	pane, store := pickerPane(t, "tdd", "grill")

	selectRow(t, pane, "[x] grill")
	require.True(t, key(pane, xui.KeyRune, ' ', 0))
	assert.Contains(t, renderText(t, pane, 84, 40), "[ ] grill", "Space toggles like Enter")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	emitted := savePendingStep(t, pane, store)
	require.Len(t, emitted, 1)
	assert.Equal(t, []string{"tdd"}, emitted[0].Skills)
	assert.Empty(t, emitted[0].DisabledSkills, "the off mark dies with the name that left the list")
}

func TestPickerRefusesAFifthSkill(t *testing.T) {
	store := skillStore()
	store.snapshot.Items[1].Actions[0].Skills = []string{"a", "b", "c", "d"}
	store.snapshot.Items[1].Actions[0].DisabledSkills = nil
	pane := newPane(store)
	pane.SetSkills([]string{"a", "b", "c", "d", "e"})
	openPendingStepDetail(t, pane)
	selectRow(t, pane, "⚙ 1 inject_skill · skills")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	selectRow(t, pane, "[ ] e")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	text := renderText(t, pane, 84, 40)
	assert.Contains(t, text, "at most 4 skills are allowed")
	assert.Contains(t, text, "[ ] e", "the refused toggle changes nothing")
}

func TestPickerMarksAndUnchecksAnOutOfCatalogName(t *testing.T) {
	store := skillStore()
	store.snapshot.Items[1].Actions[0].Skills = []string{"tdd", "ghost"}
	store.snapshot.Items[1].Actions[0].DisabledSkills = nil
	pane := newPane(store)
	pane.SetSkills([]string{"tdd", "grill"})
	openPendingStepDetail(t, pane)

	assert.Contains(t, renderText(t, pane, 84, 40), "skills: tdd, ghost⚠",
		"the detail row highlights a name the catalog does not know")

	selectRow(t, pane, "⚙ 1 inject_skill · skills")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 84, 40), "[x] ghost ⚠ not in catalog")

	selectRow(t, pane, "ghost")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 84, 40), "[ ] ghost",
		"unchecking an unknown name keeps its row until the picker closes")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	emitted := savePendingStep(t, pane, store)
	require.Len(t, emitted, 1)
	assert.Equal(t, []string{"tdd"}, emitted[0].Skills)
}

func TestManualEntryWarnsAboutUnknownNames(t *testing.T) {
	pane, store := pickerPane(t, "tdd", "grill")

	selectRow(t, pane, "other…")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, pane.State().Editing, "the escape hatch opens the free-text editor")
	clearTextField(t, pane)
	paste(pane, "tdd ghost")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	text := renderText(t, pane, 84, 40)
	assert.Contains(t, text, "not in the skill catalog: ghost",
		"an unknown name saves with a warning, never silently")
	assert.Contains(t, text, "[x] ghost ⚠ not in catalog",
		"the typed name gets a picker row at once")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	emitted := savePendingStep(t, pane, store)
	require.Len(t, emitted, 1)
	assert.Equal(t, []string{"tdd", "ghost"}, emitted[0].Skills)
}

func TestPickerJumpFiltersTheCatalog(t *testing.T) {
	pane, _ := pickerPane(t, "code-review", "tdd", "grill", "wireframe", "postmortem")

	require.True(t, key(pane, xui.KeyRune, '/', 0))
	require.True(t, pane.State().Jumping, "the skills picker is the one choice list with the jump")
	for _, r := range "post" {
		require.True(t, key(pane, xui.KeyRune, r, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.True(t, selectedRowContains(t, pane, "postmortem"),
		"the jump lands the real selection on the tightest match")
}

func TestNoCatalogKeepsTheTextEditor(t *testing.T) {
	store := skillStore()
	pane := newPane(store)
	openPendingStepDetail(t, pane)
	selectRow(t, pane, "⚙ 1 inject_skill · skills")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	assert.True(t, pane.State().Editing,
		"no catalog installed: the free-text editor stays the entry path")
}

// TestPickerSurvivesUndo pins the undo granularity: one toggle is one logical
// edit, and taking it back restores the previous checked set.
func TestPickerSurvivesUndo(t *testing.T) {
	pane, _ := pickerPane(t, "code-review", "tdd", "grill")

	selectRow(t, pane, "code-review")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyEscape, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 'z', xui.ModCtrl))
	assert.Contains(t, renderText(t, pane, 84, 40), "skills: tdd, grill (off)",
		"undo takes back the toggle as one edit")
}
