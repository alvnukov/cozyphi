package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

func TestPaneTogglesOpenCodeIntegration(t *testing.T) {
	store := fixtureStore()
	store.snapshot.OpenCodeEnabled = true
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, drawText(pane), "[x] OpenCode integration")

	clickRow(t, pane, "[x] OpenCode integration")
	assert.True(t, pane.State().Dirty)
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.False(t, store.applied[0].OpenCodeEnabled)
}
