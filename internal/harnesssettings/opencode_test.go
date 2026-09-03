package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
)

func TestManagerOpenCodeDefaultsEnabledAndPersistsFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("opencode:\n  future: keep\n"), 0o600))
	manager, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.True(t, manager.Snapshot().OpenCodeEnabled)

	draft := manager.Snapshot().Draft()
	draft.OpenCodeEnabled = false
	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.False(t, applied.OpenCodeEnabled)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "opencode:\n  future: keep\n  enabled: false")

	reopened, err := harnesssettings.Open(path, mustRuntime(t), nil)
	require.NoError(t, err)
	assert.False(t, reopened.Snapshot().OpenCodeEnabled)
}
