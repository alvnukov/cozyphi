package statuspane_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

type store struct {
	applied []harnesssettings.Draft
	err     error
}

func (*store) Snapshot() harnesssettings.Snapshot {
	return harnesssettings.Snapshot{Plan: plangate.DefaultDefaults(), OpenCodeEnabled: true}
}

func (s *store) Apply(_ context.Context, draft harnesssettings.Draft) (harnesssettings.Snapshot, error) {
	s.applied = append(s.applied, draft)
	return s.Snapshot(), s.err
}

func TestEmbeddedConfigEditsSavesAndRetainsFailedDraft(t *testing.T) {
	s := &store{err: errors.New("disk full")}
	config := settings.New(components.DefaultTheme(), s, nil)
	p := statuspane.New(components.DefaultTheme(), config, nil, nil, nil)
	p.ConfigureTabs(func() string { return statuspane.Config }, nil)
	p.Show(statuspane.Snapshot{})
	press(p, xui.KeyTab, 0)
	root := p.Draw(components.DrawContext{Max: components.Size{Width: 100, Height: 45}, Method: xui.WidthUnicode})
	require.Len(t, root.Children, 1)
	child := root.Children[0]
	rows := strings.Split(components.SurfaceText(child.Surface), "\n")
	clicked := false
	for y, row := range rows {
		if x := strings.Index(row, "[x] OpenCode integration"); x >= 0 {
			p.HandleEvent(&components.EventContext{}, xui.MouseEvent{
				Action: xui.MousePress, Button: xui.MouseLeft,
				X: x, Y: y + child.Origin.Y,
			})
			clicked = true
			break
		}
	}
	require.True(t, clicked, "editable setting remains inside the dashboard")
	require.True(t, config.State().Dirty)
	save := xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl}
	p.HandleEvent(&components.EventContext{}, save)
	require.Len(t, s.applied, 1)
	assert.False(t, s.applied[0].OpenCodeEnabled)
	assert.True(t, p.Visible())
	assert.Contains(t, config.State().Error, "disk full")
	s.err = nil
	p.HandleEvent(&components.EventContext{}, save)
	require.Len(t, s.applied, 2)
	assert.False(t, s.applied[1].OpenCodeEnabled)
	assert.False(t, p.Visible())
}
