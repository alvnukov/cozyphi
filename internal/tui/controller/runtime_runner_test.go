package controller

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
)

func TestRuntimeBoundJobRunnerFreezesResolvedRoleConfiguration(t *testing.T) {
	proj := writeAgentConfig(t, "    explore: zai-coding-plan/glm-4.5-air\n")
	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close()) })
	ws, err := rt.Workspace(proj.Root())
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "")
	require.NoError(t, err)
	c.providers = connectedProviderManager(t)

	bind := func() agent.EngineRunner {
		t.Helper()
		runner, ok := c.bindJobRunner(c.modelCfg, c.Hooks(), nil).(agent.EngineRunner)
		require.True(t, ok)
		require.Nil(t, runner.ModelFn, "bound runners must not read the controller's later model")
		require.Nil(t, runner.HooksFn, "bound runners must not read later hooks")
		return runner
	}
	old := bind()
	pinned, ok := old.ModelForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, "zai-coding-plan/glm-4.5-air", pinned.Name)
	require.Equal(t, "k", pinned.APIKey)
	_, ok = old.ModelForRole(job.RoleWorker)
	require.False(t, ok)

	// Replace the connected catalog offline: the same pin now resolves to
	// different limits. Keeping only the pin's name would not freeze a job.
	require.NoError(t, c.providers.ReplaceCatalog(strings.NewReader(`{
  "zai-coding-plan": {
    "id": "zai-coding-plan", "name": "Z.AI Coding Plan",
    "api": "https://api.z.ai/api/coding/paas/v4", "npm": "@ai-sdk/openai-compatible",
    "models": {"glm-4.5-air": {
      "id": "glm-4.5-air", "name": "changed catalog model", "tool_call": true,
      "limit": {"context": 65432, "output": 4321}
    }}
  }
}`)))
	catalogRunner := bind()
	updated, ok := catalogRunner.ModelForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, pinned.Name, updated.Name)
	require.NotEqual(t, pinned, updated)
	stillPinned, ok := old.ModelForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, pinned, stillPinned, "a bound runner must retain the resolved configuration")

	// Reload actual project configuration, moving the pin to another role.
	require.NoError(t, os.WriteFile(c.proj.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: replacement-key
agents:
  models:
    worker: m
`), 0o600))
	require.NoError(t, c.proj.LoadConfig())
	fresh := bind()
	_, ok = fresh.ModelForRole(job.RoleExplore)
	require.False(t, ok, "newly unpinned roles must inherit")
	worker, ok := fresh.ModelForRole(job.RoleWorker)
	require.True(t, ok)
	require.Equal(t, "m", worker.Name)
	require.Equal(t, "replacement-key", worker.APIKey)

	stillPinned, ok = old.ModelForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, pinned, stillPinned)
	_, ok = old.ModelForRole(job.RoleWorker)
	require.False(t, ok, "new role pins must not leak into an already bound runner")
	stillUpdated, ok := catalogRunner.ModelForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, updated, stillUpdated)
	name, ok := old.ModelNameForRole(job.RoleExplore)
	require.True(t, ok)
	require.Equal(t, pinned.Name, name, "spawn metadata and execution must use the same snapshot")
}
