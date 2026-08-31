package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutateUIStateRoundTripIsOwnerOnly(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	require.NoError(t, MutateUIState(global, func(s *UIState) {
		s.SidebarWidth = 41
		s.SidebarHidden = true
	}))

	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Equal(t, 41, got.SidebarWidth)
	assert.False(t, got.SidebarVisible())

	info, err := os.Stat(global.UIStateFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestMutateUIStatePreservesUntouchedSiblings(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	require.NoError(t, MutateUIState(global, func(s *UIState) {
		s.SidebarWidth = 41
		s.StopLimitDisabled = true
	}))

	// A later cycle that touches only visibility must keep both siblings.
	require.NoError(t, MutateUIState(global, func(s *UIState) { s.SidebarHidden = true }))

	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Equal(t, 41, got.SidebarWidth)
	assert.True(t, got.StopLimitDisabled)
	assert.False(t, got.SidebarVisible())
}

func TestLoadUIStateMissingDefaultsSidebarVisible(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Zero(t, got.SidebarWidth)
	assert.True(t, got.SidebarVisible())

	require.NoError(t, os.WriteFile(filepath.Join(global.Root(), "ui.json"), []byte("{"), 0o600))
	_, err = LoadUIState(global)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse UI preferences")
}

func TestPlanEnabledDefaultsOnAndSurvivesRoundTrip(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.True(t, got.PlanEnabled(), "a missing UI state file enables the plan without migration")

	require.NoError(t, MutateUIState(global, func(s *UIState) { s.PlanDisabled = true }))
	got, err = LoadUIState(global)
	require.NoError(t, err)
	assert.False(t, got.PlanEnabled())
	assert.True(t, got.StopLimitEnabled(), "switching the plan off leaves sibling preferences alone")
}

func TestUIStateLastModelRoundTrip(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	require.NoError(t, MutateUIState(global, func(s *UIState) {
		s.LastModel = "claude-sonnet"
	}))

	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet", got.LastModel)
}
