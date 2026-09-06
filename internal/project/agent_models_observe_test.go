package project

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// A pin that no longer names a model does not fail a spawn — the child runs
// the session's model instead. The three cases therefore look identical from
// outside, and telling them apart is the whole of why "the model I pinned is
// not the one it ran" is answerable at all.
func TestObserveSeparatesAResolvedPinFromAStaleOneAndFromNoPinAtAll(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
  - name: cheap
    api_key: k
agents:
  models:
    explore: cheap
    worker: no-such-model
`), 0o644))
	require.NoError(t, p.LoadConfig())

	pins := p.Config().AgentModels(nil).Observe()

	assert.Equal(t, []diag.RolePin{
		{Role: "explore", Ref: "cheap", Model: "cheap", Resolved: true},
		{Role: "worker", Ref: "no-such-model"},
		{Role: "review"},
	}, pins, "every role answers, in the canonical order, whether or not it was pinned")
}

// The resolver is the one that decides what a pin becomes, so the view asks
// it rather than reading the raw names — a name that only the live catalog
// knows resolves here exactly as it does for a spawn.
func TestObserveResolvesThroughTheSameCatalogASpawnWouldUse(t *testing.T) {
	p := discoverInTempHome(t)
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(`
models:
  - name: m
    api_key: k
agents:
  models:
    explore: vendor/from-catalog
`), 0o644))
	require.NoError(t, p.LoadConfig())
	cfg := p.Config()

	assert.Equal(t, diag.RolePin{Role: "explore", Ref: "vendor/from-catalog"},
		cfg.AgentModels(nil).Observe()[0],
		"the static config alone cannot resolve a catalog name, and says so rather than guessing")

	catalog := func(name string) (llm.ModelConfig, bool) {
		if name == "vendor/from-catalog" {
			return llm.ModelConfig{Name: name, APIKey: "k"}, true
		}
		return cfg.FindModel(name)
	}
	assert.Equal(t, diag.RolePin{
		Role: "explore", Ref: "vendor/from-catalog", Model: "vendor/from-catalog", Resolved: true,
	}, cfg.AgentModels(catalog).Observe()[0])
}

// A configuration nobody loaded pins nothing, which is a different answer
// from pinning something that does not resolve.
func TestObserveOnAnUnloadedConfigurationReportsNoPinsRatherThanStaleOnes(t *testing.T) {
	var cfg *Config

	pins := cfg.AgentModels(nil).Observe()

	require.Len(t, pins, 3)
	for _, pin := range pins {
		assert.Empty(t, pin.Ref, pin.Role)
		assert.False(t, pin.Resolved, pin.Role)
	}
}
