package planedit_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestHumanModelPickerReplacesAuthoredEffort(t *testing.T) {
	for _, choice := range []string{"default", "low", "clear"} {
		t.Run(choice, func(t *testing.T) {
			store := actionStore()
			store.snapshot.Items[1].Effort = "high"
			store.efforts = map[string][]string{"plan-b": {"low", "high"}}
			pane := newPane(store)
			openPendingStepDetail(t, pane)
			require.Contains(t, renderText(t, pane, 100, 40), "Model: (type default) · high")
			selectRow(t, pane, "Model:")
			require.True(t, key(pane, xui.KeyEnter, 0, 0))
			want := ""
			if choice == "clear" {
				selectRow(t, pane, "(type default)")
			} else {
				selectRow(t, pane, "plan-b")
				require.True(t, key(pane, xui.KeyEnter, 0, 0))
				selectRow(t, pane, choice)
				want = "plan-b"
				if choice == "low" {
					want += ":low"
				}
			}
			require.True(t, key(pane, xui.KeyEnter, 0, 0))
			require.NotContains(t, renderText(t, pane, 100, 40), "· high")
			require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
			require.Len(t, store.applied, 1)
			updates := findOps(store.applied[0].ops, session.PlanPatchUpdateStep)
			require.Len(t, updates, 1)
			require.True(t, updates[0].Effort.Set)
			require.Empty(t, updates[0].Effort.Value)
			require.Equal(t, want, updates[0].Model.Value)
		})
	}
}

func TestPanePreservesUntouchedAuthoredEffort(t *testing.T) {
	store := actionStore()
	store.snapshot.Items[1].Model = "plan-b:low"
	store.snapshot.Items[1].Effort = "high"
	pane := newPane(store)
	openPendingStepDetail(t, pane)
	require.Contains(t, renderText(t, pane, 100, 40), "Model: plan-b · high")
	selectRow(t, pane, "Model:")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Empty(t, store.applied)
	require.Equal(t, "high", store.snapshot.Items[1].Effort)
}
