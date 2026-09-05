package statuspane_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestConfigUsesDashboardNavigation(t *testing.T) {
	p := pane()
	p.ConfigureTabs(func() string { return statuspane.Config }, nil)
	p.Show(statuspane.Snapshot{})
	press(p, xui.KeyTab, 0)
	require.Equal(t, statuspane.Usage, p.Tab())
	press(p, xui.KeyLeft, 0)
	assert.Equal(t, statuspane.Config, p.Tab())
	press(p, xui.KeyF2, 0)
	assert.Equal(t, statuspane.Status, p.Tab())
	press(p, xui.KeyF3, 0)
	assert.Equal(t, statuspane.Config, p.Tab())
	press(p, xui.KeyRight, 0)
	assert.Equal(t, statuspane.Usage, p.Tab())
}
