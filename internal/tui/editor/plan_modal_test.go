package editor

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

func planModalContract() session.PlanV2 {
	return session.PlanV2{
		Goal:            "sidebar-sync",
		Approach:        "edit the plan in the modal and let the bus carry the result",
		SuccessCriteria: []string{"the plan view shows what was committed"},
		Constraints:     []string{"the pane never writes to the shell"},
		WorkingContext:  "the controller owns the write path",
		Items: []session.PlanItem{{
			ID:       "wire-pane",
			Content:  "wire the pane into the shell",
			Type:     session.StepEdit,
			Status:   session.PlanPending,
			Why:      "a hidden pane is dead code",
			DoneWhen: "Ctrl+P opens the editor",
		}},
	}
}

// seedPlanSession writes a session file holding contract, so an editor resuming
// it starts with a real plan behind the modal.
func seedPlanSession(t *testing.T, contract session.PlanV2) string {
	t.Helper()
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = manager.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	return manager.File()
}

func paneKey(e *Editor, code xui.KeyCode, r rune, mods xui.Modifiers) bool {
	return e.planPane.HandleEvent(
		&components.EventContext{},
		xui.KeyEvent{Press: true, Code: code, Rune: r, Mods: mods},
	)
}

// TestEditorPlanModalApplyRefreshesThePlanViewThroughTheBus pins the mechanism
// that carries a modal apply to the plan view, so nobody re-adds a callback
// from the pane back into the shell. The pane knows only its Store: the write
// lands in Controller.PatchPlan, which publishes PlanUpdatedMsg, and the next
// bus drain hands that snapshot to Sidebar.SetPlan. The sidebar is therefore
// still on the old goal the instant the modal closes — that assertion is the
// guard: it fails the moment the pane starts telling the shell directly.
func TestEditorPlanModalApplyRefreshesThePlanViewThroughTheBus(t *testing.T) {
	e := newTestEditorResuming(t, t.TempDir(), t.TempDir(), seedPlanSession(t, planModalContract()))
	require.Contains(t, sidebarText(e), "sidebar-sync", "the resumed plan is on screen before the edit")

	e.planPane.Show()
	require.True(t, paneKey(e, xui.KeyEnter, 0, 0)) // Goal is selected first.
	e.planPane.HandleEvent(&components.EventContext{}, xui.PasteEvent{Text: " via-bus"})
	require.True(t, paneKey(e, xui.KeyEnter, 0, 0))

	require.True(t, paneKey(e, xui.KeyRune, 's', xui.ModCtrl))

	require.False(t, e.planPane.Visible(), "a clean apply closes the modal")
	assert.Equal(t, "sidebar-sync via-bus", e.ctrl.Plan().Goal, "the edit reached the durable patch path")
	assert.NotContains(t, sidebarText(e), "via-bus", "the pane does not refresh the plan view itself")

	e.DrainNow()

	assert.Contains(t, sidebarText(e), "via-bus", "PlanUpdatedMsg on the bus refreshes the plan view")
}
