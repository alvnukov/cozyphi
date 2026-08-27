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

func TestPaneReordersTypeHierarchyWithMoveRows(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	selectRow(t, pane, "Move type edit down")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	selectRow(t, pane, "Move type edit up")
	require.True(t, key(pane, xui.KeyRune, ' ', 0))
	assert.True(t, pane.State().Dirty)
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	got := store.applied[0].Plan
	require.Len(t, got.Types, 5)
	assert.Equal(t, plangate.DefaultDefaults().Types[1].Name, got.Types[1].Name,
		"down then up returns the type to its original rank")
}

func TestPaneMoveDownSwapsCascadeOrder(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	selectRow(t, pane, "Move type edit down")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	got := store.applied[0].Plan
	require.Len(t, got.Types, 5)
	assert.Equal(t, session.StepRun, got.Types[1].Name)
	assert.Equal(t, session.StepEdit, got.Types[2].Name)
}
