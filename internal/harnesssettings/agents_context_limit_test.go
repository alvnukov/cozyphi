package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
)

// TestApplyAgentContextLimit pins the write side: the limit lands in
// agents.context_limit next to the palette-owned agents.enabled, unlimited (0)
// removes the key instead of writing a zero, and a fresh Open reads back what
// Apply committed.
func TestApplyAgentContextLimit(t *testing.T) {
	manager := openAgentsManager(t, "agents:\n  enabled: false\n")

	draft := manager.Snapshot().Draft()
	assert.Zero(t, draft.AgentContextLimit, "no limit configured yet")
	draft.AgentContextLimit = 32000

	snap, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, 32000, snap.AgentContextLimit)

	data, err := os.ReadFile(snap.Path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "context_limit: 32000")
	assert.Contains(t, string(data), "enabled: false", "agents.enabled belongs to the palette and must survive")

	reopened, err := harnesssettings.Open(snap.Path, mustRuntime(t), &fakePlanMigrator{})
	require.NoError(t, err)
	assert.Equal(t, 32000, reopened.Snapshot().AgentContextLimit)

	// Unlimited removes the key: the absence of a limit is not a value.
	draft = reopened.Snapshot().Draft()
	draft.AgentContextLimit = 0
	snap, err = reopened.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Zero(t, snap.AgentContextLimit)
	data, err = os.ReadFile(snap.Path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "context_limit")
}

// TestApplyAgentContextLimitInvalidRefused: a negative limit is refused and
// leaves the file alone; a non-integer agents.context_limit fails the load
// naming the key.
func TestApplyAgentContextLimitInvalidRefused(t *testing.T) {
	manager := openAgentsManager(t, "")

	draft := manager.Snapshot().Draft()
	draft.AgentContextLimit = -1
	_, err := manager.Apply(t.Context(), draft)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agents context_limit must be >= 0")

	_, err = harnesssettings.Open(
		writeConfig(t, "agents:\n  context_limit: plenty\n"), mustRuntime(t), &fakePlanMigrator{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode agents.context_limit")
}

func writeConfig(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	return path
}
