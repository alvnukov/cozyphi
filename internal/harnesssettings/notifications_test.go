package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/notify"
)

func TestManagerNotificationsDefaultsAndToggles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("opencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	// A missing section is the documented default: unfocused, platform sound.
	assert.Equal(t, notify.ModeUnfocused, manager.Snapshot().Notifications.Mode)
	assert.Equal(t, notify.DefaultSound, manager.Snapshot().Notifications.Sound)

	draft := manager.Snapshot().Draft()
	assert.True(t, draft.NotificationsEnabled())
	assert.True(t, draft.NotificationSoundEnabled())

	// Turning notifications off must not disturb the rest of the section,
	// and turning them back on restores what ran before — not the default.
	draft.Notifications.Mode = notify.ModeAlways
	draft.ToggleNotifications()
	assert.False(t, draft.NotificationsEnabled())
	draft.ToggleNotifications()
	assert.Equal(t, notify.ModeAlways, draft.Notifications.Mode,
		"re-enabling must restore the mode that ran before, not flatten it to unfocused")

	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, notify.ModeAlways, applied.Notifications.Mode)
	assert.Equal(t, notify.DefaultSound, applied.Notifications.Sound)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "notifications:\n  mode: always\n")
	assert.NotContains(t, string(data), "sound:",
		"the platform default sound stays an absent key, not a pinned name")
	assert.Contains(t, string(data), "opencode:\n  future: keep\n",
		"unrelated keys in foreign sections survive")

	reopened, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, notify.ModeAlways, reopened.Snapshot().Notifications.Mode)
	assert.Equal(t, notify.DefaultSound, reopened.Snapshot().Notifications.Sound)
}

func TestManagerNotificationsSoundRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("notifications:\n  mode: always\n  sound: Glass\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.Equal(t, "Glass", manager.Snapshot().Notifications.Sound)

	// Off persists as the documented sound key (yaml quotes it — it reads
	// as a string, not a boolean).
	draft := manager.Snapshot().Draft()
	draft.ToggleNotificationSound()
	assert.False(t, draft.NotificationSoundEnabled())
	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Empty(t, applied.Notifications.Sound)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "sound: \"off\"\n")

	// A fresh draft cannot know the name the save replaced — the two keys
	// are all the config has — so re-enabling falls back to the platform
	// default sound.
	draft = applied.Draft()
	draft.ToggleNotificationSound()
	assert.Equal(t, notify.DefaultSound, draft.Notifications.Sound)

	// Within one draft, off→on keeps a hand-written name.
	draft.Notifications.Sound = "Glass"
	draft.ToggleNotificationSound()
	draft.ToggleNotificationSound()
	assert.Equal(t, "Glass", draft.Notifications.Sound)

	draft.ToggleNotifications()
	applied, err = manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, notify.ModeOff, applied.Notifications.Mode)

	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "mode: \"off\"\n")
}

func TestManagerNotificationsInvalidModeFailsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("notifications:\n  mode: sometimes\n"), 0o600))
	_, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notifications.mode")
}
