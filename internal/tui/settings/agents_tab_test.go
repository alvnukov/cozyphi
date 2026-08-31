package settings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// openAgentsTab shows the modal and walks to the Agents tab: Plan →
// General → Agents.
func openAgentsTab(t *testing.T, store *fakeStore) *settings.Pane {
	t.Helper()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.SetModelNames([]string{"cheap", "big"})
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0), "switch to the general tab")
	require.True(t, key(pane, xui.KeyTab, 0, 0), "switch to the agents tab")
	require.Equal(t, settings.TabAgents, pane.State().Tab)
	return pane
}

func TestPaneAgentsTabPinsOneRole(t *testing.T) {
	store := fixtureStore()
	pane := openAgentsTab(t, store)

	clickRow(t, pane, "Model for explore: (inherit session model)")
	assert.Contains(t, drawText(pane), "cheap", "picker lists configured models")
	clickRow(t, pane, "cheap")
	assert.Contains(t, drawText(pane), "Model for explore: cheap")
	assert.Contains(t, drawText(pane), "Model for worker: (inherit session model)")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, map[string]string{"explore": "cheap"}, store.applied[0].AgentModels)
}

func TestPaneAgentsTabBulkPinAndPerRoleClear(t *testing.T) {
	store := fixtureStore()
	pane := openAgentsTab(t, store)

	clickRow(t, pane, "Model for all roles: (inherit session model)")
	clickRow(t, pane, "big")
	assert.Contains(t, drawText(pane), "Model for all roles: big")
	assert.Contains(t, drawText(pane), "Model for review: big")

	// selectRow walks down only; jump home first because the closed picker
	// left the selection at the bottom of the tab.
	require.True(t, key(pane, xui.KeyRune, 'g', 0))
	clickRow(t, pane, "Model for worker: big")
	clickRow(t, pane, "(inherit session model)")
	assert.Contains(t, drawText(pane), "Model for all roles: mixed")
	assert.Contains(t, drawText(pane), "Model for worker: (inherit session model)")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, map[string]string{"explore": "big", "review": "big"}, store.applied[0].AgentModels)
}

func TestPaneAgentsTabEscapeClosesPickerBeforeModal(t *testing.T) {
	store := fixtureStore()
	pane := openAgentsTab(t, store)

	clickRow(t, pane, "Model for review: (inherit session model)")
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.True(t, pane.Visible(), "Escape closes the picker, not the modal")
	assert.NotContains(t, drawText(pane), "cheap", "picker options are gone")
	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	assert.False(t, pane.Visible())
	assert.Empty(t, store.applied, "no draft was persisted")
}
