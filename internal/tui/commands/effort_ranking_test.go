package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/usage"
)

func TestEffortPickerRanksSuccessfulChoicesPerModel(t *testing.T) {
	history, err := usage.Open("")
	require.NoError(t, err)
	registry := NewCommandRegistry(history)
	levels := []string{"low", "high"}
	var pickedModel, pickedEffort string
	onPick := func(model, effort string) error { //nolint:unparam // The picker callback requires an error result.
		pickedModel, pickedEffort = model, effort
		return nil
	}
	page := func(model string) palette.PaletteCommand {
		return registry.ModelEffortPage(model, levels, onPick)
	}
	assert.Equal(t, []string{"low", "high"}, effortVerbs(page("alpha")))
	findPaletteCommand(t, page("alpha").Submenu, "model-alpha-high").Run()
	assert.Equal(t, "alpha", pickedModel)
	assert.Equal(t, "high", pickedEffort)
	assert.Equal(t, []string{"high", "low"}, effortVerbs(page("alpha")))
	assert.Equal(t, []string{"low", "high"}, effortVerbs(page("beta")))

	// Both entry points share the same effort history, not just model history.
	picker := registry.ModelPickerPage(onPick, []string{"alpha"}, func(string) []string { return levels })
	assert.Equal(t, []string{"high", "low"}, effortVerbs(picker.Submenu[0]))
	findPaletteCommand(t, picker.Submenu[0].Submenu, "model-alpha-low").Run()
	assert.Equal(t, "low", pickedEffort, "the picked level reaches the callback unchanged")
	assert.Equal(t, []string{"low", "high"}, effortVerbs(page("alpha")))
	assert.Equal(t, []string{"low", "high"}, levels, "ranking must not mutate the supplied levels")
}

func TestEffortPickerDoesNotCreditUnsuccessfulPicks(t *testing.T) {
	for _, effort := range []string{"high", "low"} {
		for _, missingCallback := range []bool{false, true} {
			t.Run(effort+"/"+map[bool]string{false: "error", true: "nil"}[missingCallback], func(t *testing.T) {
				history, err := usage.Open("")
				require.NoError(t, err)
				levels := []string{"low", "high"}
				ok := func(string, string) error { return nil }
				page := ModelEffortPage("alpha", levels, ok, history)
				findPaletteCommand(t, page.Submenu, "model-alpha-low").Run()
				failed := func(string, string) error { return errors.New("rejected") }
				if missingCallback {
					failed = nil
				}
				page = ModelEffortPage("alpha", levels, failed, history)
				findPaletteCommand(t, page.Submenu, "model-alpha-"+effort).Run()
				assert.Equal(t, []string{"low", "high"}, effortVerbs(
					ModelEffortPage("alpha", levels, ok, history),
				))
				count, _ := history.Seen(usage.Models, "alpha")
				assert.Equal(t, 1, count)
			})
		}
	}
}

func TestEffortPickerWithoutHistoryKeepsCatalogOrder(t *testing.T) {
	page := ModelEffortPage("alpha", []string{"high", "low"}, func(string, string) error { return nil }, nil)
	findPaletteCommand(t, page.Submenu, "model-alpha-low").Run()
	assert.Equal(t, []string{"high", "low"}, effortVerbs(page))
}

func effortVerbs(page palette.PaletteCommand) []string {
	verbs := make([]string, 0, len(page.Submenu))
	for _, choice := range page.Submenu {
		verbs = append(verbs, choice.Verb)
	}
	return verbs
}
