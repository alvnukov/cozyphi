package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
)

// writeAgentConfig writes a project config with agents.models pins pointing at
// a connected-catalog name (zai-coding-plan/glm-4.5-air) and returns the loaded
// project.
func writeAgentConfig(t *testing.T, pins string) *project.Project {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(proj.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  models:
`+pins), 0o644))
	require.NoError(t, proj.LoadConfig())
	return proj
}

func connectedProviderManager(t *testing.T) *provider.Manager {
	t.Helper()
	dir := t.TempDir()
	mgr, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(dir, "providers.json"),
		CredentialsPath: filepath.Join(dir, "credentials.json"),
	})
	require.NoError(t, err)

	var zai provider.Info
	for _, p := range mgr.Providers() {
		if p.ID == "zai-coding-plan" {
			zai = p
		}
	}
	require.NotEmpty(t, zai.Models, "zai-coding-plan builtin must carry models")
	require.NoError(t, mgr.Connect(provider.ConnectRequest{
		ProviderID:      "zai-coding-plan",
		ExpectedBaseURL: zai.BaseURL,
		APIKey:          "k",
	}))
	return mgr
}

// TestControllerAgentModelResolvesConnectedCatalogPin verifies that a pin
// referencing a connected-catalog model (providerID/modelID) resolves and does
// not warn: the pin picker offers such names, so resolution must accept them,
// not treat them as static-config-only.
func TestControllerAgentModelResolvesConnectedCatalogPin(t *testing.T) {
	proj := writeAgentConfig(t, "    explore: zai-coding-plan/glm-4.5-air\n")
	ctrl := &Controller{proj: proj, providers: connectedProviderManager(t)}

	mc, ok := ctrl.agentModelFor(job.RoleExplore)
	require.True(t, ok, "a connected-catalog pin must resolve")
	assert.Equal(t, "zai-coding-plan/glm-4.5-air", mc.Name)
	assert.Equal(t, "k", mc.APIKey)

	assert.Empty(t, ctrl.AgentModelWarnings(), "a live catalog pin must not warn")
}

// TestControllerAgentModelWarnsOnUnresolvedPin covers both kinds of dead pin:
// a name absent from the connected catalog and a model of a provider that was
// never connected. Both degrade to inheritance and warn; a live catalog pin and
// an unset role stay silent.
func TestControllerAgentModelWarnsOnUnresolvedPin(t *testing.T) {
	proj := writeAgentConfig(t, `    explore: zai-coding-plan/glm-4.5-air
    worker: codex/gpt-5.5
    review: zai-coding-plan/does-not-exist
`)
	ctrl := &Controller{proj: proj, providers: connectedProviderManager(t)}

	mc, ok := ctrl.agentModelFor(job.RoleExplore)
	require.True(t, ok)
	assert.Equal(t, "zai-coding-plan/glm-4.5-air", mc.Name)
	assert.Equal(t, "k", mc.APIKey)

	_, ok = ctrl.agentModelFor(job.RoleWorker)
	assert.False(t, ok, "a model of an unconnected provider must inherit, not error")
	_, ok = ctrl.agentModelFor(job.RoleReview)
	assert.False(t, ok, "a stale catalog name must inherit, not error")

	assert.Equal(t, []string{"worker=codex/gpt-5.5", "review=zai-coding-plan/does-not-exist"},
		ctrl.AgentModelWarnings(), "only dead pins warn; a live catalog pin and an unset role do not")
}
