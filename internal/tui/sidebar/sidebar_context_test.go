package sidebar

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// TestSidebarContextEntry pins the session-only context rows: they render on
// the settings tab, a click opens a digit entry, Enter commits through the
// controller callback (never persisting), and an empty commit restores the
// default/unlimited value.
func TestSidebarContextEntry(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.setTab(tabSettings)

	var gotMain, gotAgents int
	s.ConfigureContext(128000, 0, func(tokens int) error {
		gotMain = tokens
		return nil
	}, func(tokens int) error {
		gotAgents = tokens
		return nil
	})

	text := drawText(s, 24)
	require.Contains(t, text, "window 128k")
	require.Contains(t, text, "agents ∞", "no agent limit configured: unlimited")

	// Click the window row: the entry opens empty.
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: s.mainCtxRowY,
	})
	require.True(t, s.mainCtxEntry)
	assert.Contains(t, drawText(s, 24), "window [_]")

	// Digits land in the buffer; Enter commits and the controller's answer
	// refreshes the displayed value.
	ctx := &components.EventContext{}
	for _, r := range "64000" {
		handled, err := s.HandleSettingsKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
		require.NoError(t, err)
		require.True(t, handled)
	}
	handled, err := s.HandleSettingsKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.NoError(t, err)
	require.True(t, handled)
	assert.False(t, s.mainCtxEntry)
	assert.Equal(t, 64000, gotMain)

	s.SetContextWindow(64000)
	assert.Contains(t, drawText(s, 24), "window 64k")

	// The agents row: an empty Enter commits 0 — unlimited again.
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: s.agentsCtxRowY,
	})
	require.True(t, s.agentsCtxEntry)
	handled, err = s.HandleSettingsKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, 0, gotAgents)

	s.SetAgentsContext(32000)
	text = drawText(s, 24)
	assert.Contains(t, text, "agents 32k")
	assert.Contains(t, text, "window 64k", "the main window row survives next to it")

	// Escape cancels without committing.
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: s.mainCtxRowY,
	})
	require.True(t, s.mainCtxEntry)
	_, err = s.HandleSettingsKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: '9'})
	require.NoError(t, err)
	handled, err = s.HandleSettingsKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	require.NoError(t, err)
	require.True(t, handled)
	assert.False(t, s.mainCtxEntry)
	assert.Equal(t, 64000, gotMain, "Escape committed nothing")
}
