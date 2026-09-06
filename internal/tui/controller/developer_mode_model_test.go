package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

// twoModelConfig is a configuration file naming two models, with defaultName
// as the default. Writing it twice with different defaults is how this file
// changes the configuration on disk under a running process.
func twoModelConfig(defaultName string) string {
	config := "models:\n"
	for _, model := range []struct {
		name   string
		window int
	}{{"alpha", 111000}, {"beta", 222000}} {
		config += "  - name: " + model.name + "\n" +
			"    api_name: " + model.name + "-wire\n" +
			"    protocol: openai\n" +
			"    api_key: config-file-secret\n" +
			"    base_url: http://127.0.0.1:9\n"
		switch model.window {
		case 111000:
			config += "    context_window: 111000\n"
		default:
			config += "    context_window: 222000\n"
		}
		if model.name == defaultName {
			config += "    default: true\n"
		}
	}
	return config
}

// developerModelRuntime starts a granted developer process against a real
// configuration file — no environment override — so the configured layer has
// a config file to name as its origin. It returns the runtime, a session and
// the path of the file the process loaded.
func developerModelRuntime(t *testing.T) (*Controller, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	cwd := t.TempDir()

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(twoModelConfig("alpha")), 0o600))

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, configFile
}

// modelField observes one field of the model category through the session's
// own registry, the way the harness tool does.
func modelField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryModel, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheConfiguredModelIsWhatWasLoaded_NotWhatIsOnDiskNow(t *testing.T) {
	c, configFile := developerModelRuntime(t)

	loaded := modelField(t, c, diag.KeyModelName)
	require.Equal(t, diag.StatePresent, loaded.Configured.State)
	assert.Equal(t, "alpha", loaded.Configured.Value.Str)
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "models[].default"}, loaded.Configured.Source)
	assert.Equal(t, "alpha", loaded.Loaded.Value.Str)
	assert.Equal(t, "alpha", loaded.Effective.Value.Str, "the session runs on what it loaded")
	require.Equal(t, int64(111000), modelField(t, c, diag.KeyModelContextWindow).Effective.Value.Int)

	// The file changes under the running process. Nothing observes it into
	// existence: an observation reads, it never reloads.
	require.NoError(t, os.WriteFile(configFile, []byte(twoModelConfig("beta")), 0o600))

	afterDisk := modelField(t, c, diag.KeyModelName)
	assert.Equal(t, "alpha", afterDisk.Configured.Value.Str,
		"the configured layer is what the loader resolved, not what the file says now")
	assert.Equal(t, "alpha", afterDisk.Loaded.Value.Str)
	assert.Equal(t, "alpha", afterDisk.Effective.Value.Str)
	assert.Equal(t, loaded.Revision, afterDisk.Revision, "the engine went through no generation")

	// A session-time pick is a different matter: it moves the engine, and the
	// two layers part company with the reason named.
	require.NoError(t, c.SetModel("beta"))

	afterPick := modelField(t, c, diag.KeyModelName)
	assert.Equal(t, "alpha", afterPick.Configured.Value.Str, "the loader's answer is still the loader's answer")
	assert.Equal(t, diag.SourceConfigFile, afterPick.Configured.Source.Kind)
	assert.Equal(t, "beta", afterPick.Loaded.Value.Str)
	assert.Equal(t, diag.SourceSession, afterPick.Loaded.Source.Kind)
	assert.Equal(t, "beta", afterPick.Effective.Value.Str)
	assert.Equal(t, diag.SourceSession, afterPick.Effective.Source.Kind)
	assert.NotEqual(t, afterDisk.Revision, afterPick.Revision, "a swap is a new model generation")

	window := modelField(t, c, diag.KeyModelContextWindow)
	assert.Equal(t, int64(111000), window.Configured.Value.Int)
	assert.Equal(t, int64(222000), window.Effective.Value.Int)
}

func TestTheObservedModelNeverCarriesTheCredentialsItRunsOn(t *testing.T) {
	c, _ := developerModelRuntime(t)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryModel)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)

	rendered := snapshot.JSON()
	assert.NotContains(t, rendered, "config-file-secret", "the key the session authenticates with stays put")
	assert.NotContains(t, rendered, "127.0.0.1:9", "so does the endpoint it talks to")
	assert.Contains(t, rendered, "alpha")
}

func TestTheTUIPublishesTheWholeModelCategory(t *testing.T) {
	c, _ := developerModelRuntime(t)

	var entry diag.CatalogEntry
	for _, candidate := range c.diagnostics.Catalog().Categories {
		if candidate.Category == diag.CategoryModel {
			entry = candidate
		}
	}

	require.Equal(t, diag.CategoryModel, entry.Category, "the TUI registers the collector too")
	assert.Equal(t, diag.AvailabilityAvailable, entry.Availability)
	assert.Equal(t, []string{
		"name", "request_name", "protocol", "effort", "effort.request", "effort.levels",
		"context_window", "max_output_tokens", "variants", "options", "thinking",
		"pinned_by_plan", "source_order",
	}, entry.Keys, "the same key set the headless entry point declares")
	assert.Contains(t, entry.Reason, "api keys")
}
