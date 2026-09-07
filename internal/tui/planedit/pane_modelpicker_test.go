package planedit_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
	"github.com/alvnukov/cozyphi/internal/usage"
)

func (s *fakeStore) ModelPicker(onPick func(string, string) error) palette.PaletteCommand {
	return commands.NewBuiltinRegistry().ModelPickerPage(onPick, s.models, s.ModelEfforts)
}

type sharedPickerStore struct {
	*fakeStore
	registry *commands.CommandRegistry
}

func (s sharedPickerStore) ModelPicker(onPick func(string, string) error) palette.PaletteCommand {
	return s.registry.ModelPickerPage(onPick, s.models, s.ModelEfforts)
}

func TestPlanUsesSharedPickerPage(t *testing.T) {
	history, err := usage.Open("")
	require.NoError(t, err)
	require.NoError(t, history.Record(usage.Models, "plan-b"))
	store := sharedPickerStore{actionStore(), commands.NewBuiltinRegistry(history)}
	pane := planedit.New(components.DefaultTheme(), store, nil)
	pane.Show()
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	text := renderText(t, pane, 100, 40)
	require.Less(
		t,
		strings.Index(text, "plan-b"),
		strings.Index(text, "plan-a"),
		"render the shared page order, not Store.Models",
	)
	require.Empty(t, store.applied)
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	require.Contains(t, renderText(t, pane, 100, 40), "explore: plan-a")
	require.False(t, pane.State().Dirty)
	require.Equal(t, "plan-b", store.registry.RankModels(store.models)[0], "cancel records no choice")
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "plan-a")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.Equal(t, "plan-a", store.registry.RankModels(store.models)[0], "shared callbacks record completed choices")
	require.Empty(t, store.applied)
	selectRow(t, pane, "explore:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "(type default)")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.Equal(t, []string{"plan-a", "plan-b"}, store.registry.RankModels(store.models))
	require.Empty(t, store.applied)
}

func TestSharedPickerPreservesDraftDefaultAndBack(t *testing.T) {
	for _, step := range []bool{false, true} {
		t.Run(map[bool]string{false: "type", true: "step"}[step], func(t *testing.T) {
			store := actionStore()
			store.efforts = map[string][]string{"plan-b": {"high"}}
			pane := newPane(store)
			open := func() {
				if step {
					selectRow(t, pane, "Model:")
				} else {
					selectRow(t, pane, "explore:")
				}
				require.True(t, key(pane, xui.KeyEnter, 0, 0))
			}
			if step {
				openPendingStepDetail(t, pane)
			}
			open()
			selectRow(t, pane, "plan-b")
			require.True(t, key(pane, xui.KeyEnter, 0, 0))
			require.True(t, selectedRowContains(t, pane, "high"))
			require.False(t, pane.State().Dirty, "opening effort does not change the draft")
			require.True(t, key(pane, xui.KeyEscape, 0, 0))
			require.True(t, selectedRowContains(t, pane, "type default"))
			require.True(t, key(pane, xui.KeyEscape, 0, 0))
			require.False(t, pane.State().Dirty, "cancel changes neither pin nor draft")
			open()
			selectRow(t, pane, "plan-b")
			require.True(t, key(pane, xui.KeyEnter, 0, 0))
			require.True(t, key(pane, xui.KeyEnter, 0, 0)) // the model's own level completes the pick
			require.True(t, pane.State().Dirty)
			require.Empty(t, store.applied, "a completed choice is still only a draft")
			text := renderText(t, pane, 100, 40)
			require.Contains(t, text, "plan-b")
			require.Contains(t, text, "high", "the committed pin carries the picked level")
			open()
			selectRow(t, pane, "(type default)")
			require.True(t, key(pane, xui.KeyEnter, 0, 0))
			require.Contains(t, renderText(t, pane, 100, 40), "(type default)")
			require.Empty(t, store.applied)
			require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
			if step {
				require.Empty(t, store.applied, "restoring the original default leaves no patch")
			} else {
				require.Len(t, store.applied, 1)
			}
		})
	}
}
