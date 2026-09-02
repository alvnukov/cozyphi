package settings_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

func TestPaneTogglesSystemNotifications(t *testing.T) {
	store := fixtureStore()
	store.snapshot.Notifications = harnesssettings.Notifications{Mode: notify.ModeAlways, Sound: "Glass"}
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, drawText(pane), "[x] System notifications")
	assert.Contains(t, drawText(pane), "[x] Notification sound")

	clickRow(t, pane, "[x] System notifications")
	assert.True(t, pane.State().Dirty)
	assert.Contains(t, drawText(pane), "[ ] System notifications")

	// Re-enabling restores the hand-written "always", not the default
	// "unfocused" — a checkbox must not flatten the config.
	clickRow(t, pane, "[ ] System notifications")
	assert.Contains(t, drawText(pane), "[x] System notifications")

	clickRow(t, pane, "[x] Notification sound")
	assert.Contains(t, drawText(pane), "[ ] Notification sound")

	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))
	require.Len(t, store.applied, 1)
	assert.Equal(t, notify.ModeAlways, store.applied[0].Notifications.Mode)
	assert.Empty(t, store.applied[0].Notifications.Sound)
}

func TestPaneNotificationCheckboxesDefaultOn(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	require.True(t, key(pane, xui.KeyTab, 0, 0))
	assert.Contains(t, drawText(pane), "[x] System notifications")
	assert.Contains(t, drawText(pane), "[x] Notification sound")
}
