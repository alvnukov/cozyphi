package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
)

// The headless counterpart of the lenient TUI start: `cozyphi run` has no
// screen to guide anyone on, so an unresolvable model stops the run with an
// error naming every way to configure one.

func TestRequireModelNamesEveryWayToConfigure(t *testing.T) {
	p, _ := testProject(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	// LoadConfig plants the commented template: zero models, no error — the
	// refusal belongs to requireModel, not to config loading.
	require.NoError(t, p.LoadConfig())

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err, "bootstrap itself must succeed without a model")

	_, err = bs.requireModel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no model configured")
	assert.Contains(t, err.Error(), p.Global().ConfigFile(), "names editing config.yaml")
	assert.Contains(t, err.Error(), "cozyphi config")
	assert.Contains(t, err.Error(), "/connect", "names the TUI sign-in")
	assert.Contains(t, err.Error(), "COZYPHI_MODEL")
	assert.Contains(t, err.Error(), "COZYPHI_API_KEY")
}

func TestRequireModelAcceptsConfiguredModel(t *testing.T) {
	p, _ := testProject(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	require.NoError(t, os.WriteFile(
		p.Global().ConfigFile(),
		[]byte("models:\n  - name: m\n    api_key: k\n    protocol: openai\n"),
		0o644,
	))

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)

	model, err := bs.requireModel()
	require.NoError(t, err)
	assert.Equal(t, "m", model.Name)
}

// connectOpenAIProvider writes an API-key credential for the builtin OpenAI
// provider, so a /connect-style sign-in exists without any network.
func connectOpenAIProvider(t *testing.T, p *project.Project) {
	t.Helper()
	creds := map[string]any{
		"openai": map[string]any{
			"type":     "api",
			"key":      "test-key",
			"base_url": "https://api.openai.com/v1",
			"protocol": "openai",
		},
	}
	data, err := json.Marshal(map[string]any{"version": 1, "providers": creds})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p.Global().CredentialsFile(), data, 0o600))
}

// providerCatalog opens the connected-provider store the way bootstrap does,
// so tests can name catalog models without hardcoding them.
func providerCatalog(t *testing.T, p *project.Project) []llm.ModelConfig {
	t.Helper()
	providers, err := provider.Open(provider.Options{
		CachePath:       p.Global().ProviderCatalogFile(),
		CredentialsPath: p.Global().CredentialsFile(),
	})
	require.NoError(t, err)
	catalog := providers.Models()
	require.NotEmpty(t, catalog, "the connected provider store must supply models")
	return catalog
}

func TestRequireModelUsesLastModelFromUIState(t *testing.T) {
	p, _ := testProject(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	connectOpenAIProvider(t, p)
	// The last model is the catalog's tail, so the test cannot pass by
	// accidentally hitting the first-model fallback.
	catalog := providerCatalog(t, p)
	want := catalog[len(catalog)-1].Name
	require.NoError(t, project.MutateUIState(p.Global(), func(s *project.UIState) {
		s.LastModel = want
	}))
	require.NoError(t, p.LoadConfig())

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)

	model, err := bs.requireModel()
	require.NoError(t, err)
	assert.Equal(t, want, model.Name, "the remembered last model wins over the catalog fallback")
}

func TestRequireModelFallsBackToFirstCatalogModel(t *testing.T) {
	p, _ := testProject(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	connectOpenAIProvider(t, p)
	want := providerCatalog(t, p)[0].Name
	require.NoError(t, p.LoadConfig())

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)

	model, err := bs.requireModel()
	require.NoError(t, err)
	assert.Equal(t, want, model.Name, "headless falls back to the first catalog model like the TUI")
}
