package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// The task registry row reads the level and steps it on a click — write,
// ask, read, off — and a save carries the level to the store, where the
// manager writes permissions.tasks and the editor applies it live.
func TestPaneCyclesTaskRegistryAccess(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, drawText(pane), "Task registry access: write", "an unset level shows the default")

	clickRow(t, pane, "Task registry access: write")
	assert.True(t, pane.State().Dirty)
	assert.Contains(t, drawText(pane), "Task registry access: ask")
	clickRow(t, pane, "Task registry access: ask")
	assert.Contains(t, drawText(pane), "Task registry access: read")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, tasks.AccessRead, store.applied[0].Tasks)
}

func TestPaneShowsTheStoredTaskRegistryAccess(t *testing.T) {
	store := fixtureStore()
	store.snapshot.Tasks = tasks.AccessOff
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, drawText(pane), "Task registry access: off")
}
