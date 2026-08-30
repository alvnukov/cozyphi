package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// The plan-scope block: add, cycle event and type, enter skills, apply.
func TestPaneEditsPlanLevelDefaultActions(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, func() {})
	pane.Show()

	clickRow(t, pane, "[+] Add plan action")
	assert.Contains(t, drawText(pane), "Action 1: on plan_start → compact")

	clickRow(t, pane, "event: plan_start")
	assert.Contains(t, drawText(pane), "event: plan_end")

	clickRow(t, pane, "type: compact")
	assert.Contains(t, drawText(pane), "type: inject_skill")
	assert.Contains(t, drawText(pane), "skills: (none")

	clickRow(t, pane, "skills: (none")
	for _, r := range "tdd, code-review" {
		require.True(t, key(pane, xui.KeyRune, r, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, drawText(pane), "skills: tdd, code-review")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.False(t, pane.Visible(), "apply closes the pane")
	require.Len(t, store.applied, 1)
	require.Len(t, store.applied[0].Plan.Actions, 1)
	assert.Equal(t, session.PlanAction{
		Event:  session.PlanActionOnPlanEnd,
		Type:   session.PlanActionInjectSkill,
		Skills: []string{"tdd", "code-review"},
	}, store.applied[0].Plan.Actions[0])
}

// The type-scope block: a new action under a step type inherits step events,
// and the applied draft carries it on that type only.
func TestPaneEditsTypeLevelDefaultActions(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, func() {})
	pane.Show()

	clickRow(t, pane, "[+] Add action to explore")
	assert.Contains(t, drawText(pane), "Action 1: on step_start → compact")

	clickRow(t, pane, "event: step_start")
	assert.Contains(t, drawText(pane), "event: step_end")

	clickRow(t, pane, "type: compact")
	clickRow(t, pane, "skills: (none")
	for _, r := range "tdd" {
		require.True(t, key(pane, xui.KeyRune, r, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	var found *plangate.TypeDefaults
	for i, typ := range store.applied[0].Plan.Types {
		if len(typ.Actions) > 0 {
			found = &store.applied[0].Plan.Types[i]
		}
	}
	require.NotNil(t, found, "exactly one type carries the action")
	assert.Equal(t, session.StepExplore, found.Name)
	assert.Equal(t, []session.PlanAction{{
		Event:  session.PlanActionOnStepEnd,
		Type:   session.PlanActionInjectSkill,
		Skills: []string{"tdd"},
	}}, found.Actions)
}

// Remove drops the action and the placeholder row returns.
func TestPaneRemovesDefaultActions(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, func() {})
	pane.Show()

	clickRow(t, pane, "[+] Add plan action")
	assert.Contains(t, drawText(pane), "Action 1: on plan_start")

	// The action rows land above the Add row; the row walker only moves
	// down, so park the selection at the top first.
	require.True(t, key(pane, xui.KeyHome, 0, 0))
	clickRow(t, pane, "[-] Action 1: on plan_start → compact")
	assert.NotContains(t, drawText(pane), "Action 1:")
	assert.Contains(t, drawText(pane), "(none)")
}
