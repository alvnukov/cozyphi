package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// TestPaneGeneralAgentContextLimit pins the persisted agents context limit:
// activating the row opens a digit entry seeded from the draft, Enter commits
// into the draft, and Apply carries it to the store. An empty commit means
// unlimited again.
func TestPaneGeneralAgentContextLimit(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	require.Equal(t, settings.TabGeneral, pane.State().Tab)

	view := renderText(t, pane, 72, 16)
	require.Contains(t, view, "Agents context limit: unlimited", "no limit yet")

	// Row order on General: notifications, sound, opencode, tasks, threshold,
	// agents limit — index 5.
	for range 5 {
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	require.Equal(t, 5, pane.State().Selected)
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 72, 16), "Agents context limit (tokens): _", "the entry opens empty")

	for _, r := range "32000" {
		require.True(t, key(pane, xui.KeyRune, r, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 72, 16), "Agents context limit: 32000 tokens")

	// Apply hands the draft to the store with the limit set.
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, 32000, store.applied[0].AgentContextLimit)
	assert.False(t, pane.Visible(), "a successful apply closes the modal")

	// A reopened pane reads the committed limit back; clearing the entry
	// commits 0 — unlimited.
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, renderText(t, pane, 72, 16), "Agents context limit: 32000 tokens")
	for range 5 {
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyBackspace, 0, 0)) // seeded "32000" → "3200"
	for range 4 {
		require.True(t, key(pane, xui.KeyBackspace, 0, 0))
	}
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, renderText(t, pane, 72, 16), "Agents context limit: unlimited")
}
