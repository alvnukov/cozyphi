package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A first start must plant ~/.cozyphi/config.yaml from the commented template
// and still load; the lenient contract means no model and no api_key warn
// instead of failing. These tests pin that behavior.

// TestDefaultTemplateParsesToBuiltInDefaults is the template invariant: the
// file a fresh install gets must load to exactly the Config a missing file
// yields, so planting it never changes behavior by itself.
func TestDefaultTemplateParsesToBuiltInDefaults(t *testing.T) {
	dir := t.TempDir()

	templatePath := filepath.Join(dir, "template.yaml")
	require.NoError(t, os.WriteFile(templatePath, []byte(defaultConfigTemplate), 0o600))
	missingPath := filepath.Join(dir, "missing.yaml")

	fromTemplate, err := parseConfigFile(templatePath)
	require.NoError(t, err)
	fromMissing, err := parseConfigFile(missingPath)
	require.NoError(t, err)

	assert.Equal(t, fromMissing, fromTemplate)
	assert.Empty(t, fromTemplate.Models, "the template configures no model")
	assert.True(t, fromTemplate.Agents.Enabled)
	assert.True(t, fromTemplate.OpenCode.Enabled)
}

// TestLoadConfigCreatesDefaultFileOnFirstStart: a missing config file appears
// with the template body and owner-only permissions, and loading succeeds
// with zero models.
func TestLoadConfigCreatesDefaultFileOnFirstStart(t *testing.T) {
	p := discoverInTempHome(t)

	require.NoError(t, p.LoadConfig())

	path := p.Global().ConfigFile()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "the default config file must be created")
	assert.Equal(t, defaultConfigTemplate, string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	cfg := p.Config()
	assert.Empty(t, cfg.Models, "a template-only config has no models")
	assert.Empty(t, cfg.Warnings(), "a clean first start has nothing to warn about")
}

// TestLoadConfigNeverRewritesExistingConfig: whatever the user (or an earlier
// start) left in config.yaml survives a load byte for byte.
func TestLoadConfigNeverRewritesExistingConfig(t *testing.T) {
	p := discoverInTempHome(t)
	body := "# user's own config\nmodels:\n  - name: kept\n    api_key: k\n    protocol: openai\n"
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(body), 0o644))

	require.NoError(t, p.LoadConfig())

	data, err := os.ReadFile(p.Global().ConfigFile())
	require.NoError(t, err)
	assert.Equal(t, body, string(data), "an existing config file is never rewritten")
	assert.Equal(t, "kept", p.Config().Model().Name)
}

// TestLoadConfigZeroModelsLoads: no models is a valid (if useless) config —
// the TUI resolves a model elsewhere; Model() reports the lack without
// growing an entry.
func TestLoadConfigZeroModelsLoads(t *testing.T) {
	p := discoverInTempHome(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(defaultConfigTemplate), 0o600))

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	assert.Empty(t, cfg.Models)
	// Reading the default must not fabricate an entry: callers that check
	// len(Models) after Model() would otherwise see a phantom model.
	zero := cfg.Model()
	assert.Empty(t, zero.Name)
	assert.Empty(t, zero.APIKey)
	assert.Empty(t, cfg.Models, "Model() must not add an entry to Models")
}

// TestLoadConfigNamelessEntryStillErrors: an entry without a name can never
// be addressed or switched to, so it stays a hard error.
func TestLoadConfigNamelessEntryStillErrors(t *testing.T) {
	p := discoverInTempHome(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	require.NoError(t, os.WriteFile(
		p.Global().ConfigFile(),
		[]byte("models:\n  - api_key: k\n    protocol: openai\n"),
		0o600,
	))

	err := p.LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without a name")
}

// TestLoadConfigEnvOnlyStillCreatesEntry: with no file models, the COZYPHI_*
// environment is the one thing that may create the default entry — and it
// still does after the lenient-loading change.
func TestLoadConfigEnvOnlyStillCreatesEntry(t *testing.T) {
	p := discoverInTempHome(t)
	t.Setenv("COZYPHI_MODEL", "env-model")
	t.Setenv("COZYPHI_API_KEY", "env-key")
	t.Setenv("COZYPHI_BASE_URL", "https://env.example/v1")

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	require.Len(t, cfg.Models, 1)
	assert.Equal(t, "env-model", cfg.Models[0].Name)
	assert.Equal(t, "env-model", cfg.Model().Name)
	assert.Equal(t, "env-key", cfg.Model().APIKey)
	assert.Equal(t, "https://env.example/v1", cfg.Model().BaseURL)
}

// TestLoadConfigWarnsWhenTemplateCannotBeCreated: a home that cannot hold the
// template is a warning, not a failed start — env-only setups must keep
// working on a read-only global directory.
func TestLoadConfigWarnsWhenTemplateCannotBeCreated(t *testing.T) {
	p := discoverInTempHome(t)
	// A read-only global directory lets the exclusive create fail with a
	// permission error while the file stays missing (a plain parse of a
	// missing file must still succeed for the warning to be reachable).
	require.NoError(t, os.Chmod(p.Global().Root(), 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(p.Global().Root(), 0o755)
	})
	t.Setenv("COZYPHI_MODEL", "env-model")
	t.Setenv("COZYPHI_API_KEY", "env-key")

	require.NoError(t, p.LoadConfig())

	warnings := p.Config().Warnings()
	require.Len(t, warnings, 2) // the create failure plus the protocol sniff on the env model
	assert.Contains(t, warnings[0], "could not create")
	assert.Equal(t, "env-model", p.Config().Model().Name, "the env model still loads")
}
