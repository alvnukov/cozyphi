package statuspane_test

import (
	"context"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

type configStore struct{}

func (configStore) Snapshot() harnesssettings.Snapshot { return harnesssettings.Snapshot{} }
func (configStore) Apply(context.Context, harnesssettings.Draft) (harnesssettings.Snapshot, error) {
	return harnesssettings.Snapshot{}, nil
}

func TestConfigForwardsPasteSearchAndInterceptsDashboardTabs(t *testing.T) {
	config := settings.New(components.DefaultTheme(), configStore{}, nil)
	p := statuspane.New(components.DefaultTheme(), config, nil, nil, nil)
	p.ConfigureTabs(func() string { return statuspane.Config }, nil)
	p.Show(statuspane.Snapshot{})
	press(p, xui.KeyTab, 0)
	require.Equal(t, settings.TabGeneral, config.State().Tab)
	press(p, xui.KeyRune, '/')
	require.True(t, config.State().Jumping)
	ctx := &components.EventContext{}
	require.True(t, p.HandleEvent(ctx, xui.PasteEvent{Text: "scope"}))
	assert.True(t, ctx.Consume)
	assert.Equal(t, 6, config.State().Selected)
	assert.Contains(t, text(p, 80, 20), "1 match")
	press(p, xui.KeyF3, 0)
	assert.Equal(t, statuspane.Usage, p.Tab())
	press(p, xui.KeyF2, 0)
	assert.Equal(t, statuspane.Config, p.Tab())
	assert.True(t, config.State().Jumping)
	press(p, xui.KeyEnter, 0)
	assert.False(t, config.State().Jumping)
	assert.Equal(t, 6, config.State().Selected)
}
