package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
)

func openAgentsManager(t *testing.T, config string) *harnesssettings.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(path, runtime, &fakePlanMigrator{})
	require.NoError(t, err)
	return manager
}

// TestApplyAgentModels pins the write side: pins land in agents.models,
// empty entries are dropped as inherit, and the palette-owned
// agents.enabled key survives the same configfile.Edit cycle untouched.
func TestApplyAgentModels(t *testing.T) {
	manager := openAgentsManager(t, "agents:\n  enabled: false\n")

	draft := manager.Snapshot().Draft()
	assert.Empty(t, draft.AgentModels, "no pins configured yet")
	draft.AgentModels = map[string]string{"explore": "cheap", "worker": "  "}

	snap, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"explore": "cheap"}, snap.AgentModels)

	data, err := os.ReadFile(snap.Path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "explore: cheap")
	assert.NotContains(t, string(data), "worker", "an empty pin means inherit and is not written")
	assert.Contains(t, string(data), "enabled: false", "agents.enabled belongs to the palette and must survive")

	// A fresh Open reads back exactly what Apply committed.
	reopened, err := harnesssettings.Open(snap.Path, mustRuntime(t), &fakePlanMigrator{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"explore": "cheap"}, reopened.Snapshot().AgentModels)
}

// TestApplyAgentModelsUnknownRoleFailsClosed: a role key that will never
// resolve is structural, so Apply refuses it and leaves the file alone.
func TestApplyAgentModelsUnknownRoleFailsClosed(t *testing.T) {
	manager := openAgentsManager(t, "")

	draft := manager.Snapshot().Draft()
	draft.AgentModels = map[string]string{"explorer": "m"}

	_, err := manager.Apply(t.Context(), draft)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown role")

	data, err := os.ReadFile(manager.Snapshot().Path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "explorer")
}

// TestApplyAgentModelsClearRemovesSection: an empty draft removes the
// agents.models section instead of leaving a dead `models: {}` behind.
func TestApplyAgentModelsClearRemovesSection(t *testing.T) {
	manager := openAgentsManager(t, "agents:\n  models:\n    explore: cheap\n")
	assert.Equal(t, map[string]string{"explore": "cheap"}, manager.Snapshot().AgentModels)

	draft := manager.Snapshot().Draft()
	assert.Equal(t, map[string]string{"explore": "cheap"}, draft.AgentModels, "draft seeds from snapshot")
	draft.AgentModels = nil

	snap, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Empty(t, snap.AgentModels)

	data, err := os.ReadFile(snap.Path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "models:", "no pins configured should mean no models section")
	assert.NotContains(t, string(data), "agents:", "an emptied agents section is pruned, not left as `agents: {}`")
}

// TestOpenAgentModelsUnknownRoleFails mirrors the project loader: a config
// with a role key that cannot parse fails Open instead of running with a
// pin that could never take effect.
func TestOpenAgentModelsUnknownRoleFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("agents:\n  models:\n    explorer: m\n"), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	_, err = harnesssettings.Open(path, runtime, &fakePlanMigrator{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown role")
}

func mustRuntime(t *testing.T) *plangate.Runtime {
	t.Helper()
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	return runtime
}
