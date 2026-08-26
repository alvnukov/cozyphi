package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

func TestController_SidebarPreferencesStopOnLimitDefaultsTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	prefs, err := ctrl.SidebarPreferences()
	require.NoError(t, err)
	require.True(t, prefs.StopOnLimit, "stop@128 is on by default")
}

func TestController_SaveStopLimitRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	require.NoError(t, ctrl.SaveStopLimit(false))
	prefs, err := ctrl.SidebarPreferences()
	require.NoError(t, err)
	require.False(t, prefs.StopOnLimit, "disabled stop persists to ui.json")

	require.NoError(t, ctrl.SaveStopLimit(true))
	prefs, err = ctrl.SidebarPreferences()
	require.NoError(t, err)
	require.True(t, prefs.StopOnLimit, "re-enabled stop persists back")
}
