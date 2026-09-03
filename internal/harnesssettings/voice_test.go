package harnesssettings_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
)

func TestManagerSetVoiceModelWritesTheKeyAndKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("voice:\n  hotkey: ctrl+g\nopencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	require.NoError(t, manager.SetVoiceModel(t.Context(), "small"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "  stt:\n    model: small\n")
	assert.Contains(t, string(data), "hotkey: ctrl+g",
		"the rest of the voice section survives")
	assert.Contains(t, string(data), "opencode:\n  future: keep\n",
		"unrelated sections survive")

	// A second call replaces the pin instead of appending a duplicate key.
	require.NoError(t, manager.SetVoiceModel(t.Context(), "large-v3-turbo"))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "model: large-v3-turbo")
	assert.NotContains(t, string(data), "model: small")
}

func TestManagerSetVoiceModelCreatesTheSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("opencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	require.NoError(t, manager.SetVoiceModel(t.Context(), "  small  "))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "voice:\n  stt:\n    model: small\n")
}

func TestManagerSetVoiceModelRejectsEmptyNamesAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("opencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	require.Error(t, manager.SetVoiceModel(t.Context(), "   "))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, manager.SetVoiceModel(ctx, "small"), context.Canceled)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "voice:", "a refused call writes nothing")
}
