package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// The plan defaults tab shows the closed selector as one cycling row; the
// empty default renders as adaptive-minimal, never as a blank.
func TestPaneAuthoringPolicyCyclesAndApplies(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	selectRow(t, pane, "Authoring grammar: adaptive-minimal")
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	assert.Contains(t, drawText(pane), "Authoring grammar: legacy")
	require.True(t, pane.State().Dirty)

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, plangate.AuthoringLegacy, store.applied[0].Plan.AuthoringPolicy,
		"Ctrl+S persists the selector into the plan defaults draft")
}
