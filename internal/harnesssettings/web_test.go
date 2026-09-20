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

// TestManagerWebModelRoundTrip: the pin writes into the web section of the
// same config file, survives a restart (fresh Open), and leaves foreign
// sections and the rest of the web section alone.
func TestManagerWebModelRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("web:\n  enabled: true\nopencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	pin, err := manager.WebModel()
	require.NoError(t, err)
	assert.Empty(t, pin, "an unpinned web model is a state, not an error")

	require.NoError(t, manager.SetWebModel(t.Context(), "reader-4o"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "model: reader-4o")
	assert.Contains(t, string(data), "enabled: true", "the rest of the web section survives")
	assert.Contains(t, string(data), "opencode:\n  future: keep\n", "unrelated sections survive")

	reopened, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	pin, err = reopened.WebModel()
	require.NoError(t, err)
	assert.Equal(t, "reader-4o", pin)
}

// TestManagerWebModelClearRemovesTheKey: an empty name is how the pin is
// withdrawn — an unpinned web model is a real not-ready state, not a
// validation failure. The rest of the web section stays.
func TestManagerWebModelClearRemovesTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("web:\n  enabled: true\n  model: reader-4o\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	require.NoError(t, manager.SetWebModel(t.Context(), "   "))

	pin, err := manager.WebModel()
	require.NoError(t, err)
	assert.Empty(t, pin)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "reader-4o")
	assert.Contains(t, string(data), "enabled: true", "clearing the pin keeps the section")
}

// TestManagerWebModelTrimsAndRejectsBrokenValues: surrounding whitespace is
// not part of the pin, and a non-scalar web.model is a configuration error
// the caller sees — never a silent unset.
func TestManagerWebModelTrimsAndRejectsBrokenValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("opencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	require.NoError(t, manager.SetWebModel(t.Context(), "  reader-4o  "))
	pin, err := manager.WebModel()
	require.NoError(t, err)
	assert.Equal(t, "reader-4o", pin)

	require.NoError(t, os.WriteFile(path, []byte("web:\n  model: [a, b]\n"), 0o600))
	_, err = manager.WebModel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web.model")
}

// TestManagerWebModelCancellation: a cancelled context writes nothing.
func TestManagerWebModelCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("web:\n  enabled: true\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, manager.SetWebModel(ctx, "reader-4o"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "reader-4o")
}
