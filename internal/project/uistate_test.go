package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIStateRoundTripIsOwnerOnly(t *testing.T) {
	global := GlobalLayout{root: t.TempDir()}
	require.NoError(t, SaveUIState(global, UIState{SidebarWidth: 41, SidebarHidden: true}))

	got, err := LoadUIState(global)
	require.NoError(t, err)
	assert.Equal(t, 41, got.SidebarWidth)
	assert.False(t, got.SidebarVisible())

	info, err := os.Stat(global.UIStateFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
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
