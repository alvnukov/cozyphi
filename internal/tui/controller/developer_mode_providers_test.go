package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

// The sentinels below sit in the three files a connected process reads at
// startup. None of them is a thing the harness view may repeat.
const (
	catalogKeySentinel  = "sk-catalog-key-sentinel"
	importKeySentinel   = "sk-import-key-sentinel"
	catalogHostSentinel = "catalog-endpoint-sentinel.invalid"
)

// connectedDeveloperRuntime starts a granted developer process over a saved
// provider catalog, a credential store with a key in it, and an opencode
// installation the import resolves against — the shape of a real user's
// machine rather than of an empty temp directory.
func connectedDeveloperRuntime(t *testing.T, opencodeConfig string) *Controller {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	// The import's own locations are pinned to this home, so a real opencode
	// installation on the machine running the tests changes no answer here.
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(home, "opencode"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	cwd := t.TempDir()

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile,
		[]byte(twoModelConfig("alpha")+opencodeConfig), 0o600))

	endpoint := "https://" + catalogHostSentinel + "/v1"
	require.NoError(t, os.WriteFile(proj.Global().ProviderCatalogFile(), []byte(fmt.Sprintf(
		`{"version":1,"providers":[{"id":"acme","name":"Acme","base_url":%q,"protocol":"openai",
			"models":[{"id":"acme-1","name":"Acme One","context_window":128000,"max_output_tokens":8192}]}]}`,
		endpoint)), 0o600))
	require.NoError(t, os.WriteFile(proj.Global().CredentialsFile(), []byte(fmt.Sprintf(
		`{"version":1,"providers":{"acme":{"type":"api","key":%q,"base_url":%q,"protocol":"openai"}}}`,
		catalogKeySentinel, endpoint)), 0o600))

	opencodeDir := filepath.Join(home, "opencode")
	authPath := filepath.Join(home, "data", "opencode", "auth.json")
	require.NoError(t, os.MkdirAll(opencodeDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(authPath), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(opencodeDir, "opencode.json"),
		[]byte(`{"provider": {}}`), 0o600))
	require.NoError(t, os.WriteFile(authPath, []byte(fmt.Sprintf(
		`{"acme":{"type":"api","key":%q}}`, importKeySentinel)), 0o600))

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c
}

func TestTheHarnessNamesTheProviderCatalogItActuallyHolds(t *testing.T) {
	c := connectedDeveloperRuntime(t, "")

	catalog := modelField(t, c, diag.KeyModelCatalog)
	assert.Equal(t, diag.StateNotApplicable, catalog.Configured.State,
		"a catalog is fetched and cached, never configured")
	require.Equal(t, diag.StatePresent, catalog.Effective.State)
	assert.Equal(t, int64(3), catalog.Effective.Value.Int,
		"the saved provider joins the two built-in ones")
	assert.Equal(t, diag.SourceConfigFile, catalog.Effective.Source.Kind)
	assert.Contains(t, catalog.Effective.Source.Ref, "last-known-good",
		"the answer says which of the two origins it came from")
	assert.Equal(t, diag.ApplyImmediate, catalog.Apply)
	assert.Equal(t, diag.ScopeProcess, catalog.Scope)
}

func TestTheHarnessNamesTheConnectedProvidersAndNothingTheyHold(t *testing.T) {
	c := connectedDeveloperRuntime(t, "")

	connected := modelField(t, c, diag.KeyModelConnected)
	require.Equal(t, diag.StatePresent, connected.Effective.State)
	assert.Equal(t, []string{"acme"}, connected.Effective.Value.List,
		"presence under a name is the whole of what a stored credential says")

	// The session's own model came from that provider, so the credential
	// fields answer about a connection that really exists.
	credential := modelField(t, c, diag.KeyModelCredential)
	require.Equal(t, diag.StatePresent, credential.Effective.State)
	assert.True(t, credential.Effective.Value.Bool)
	assert.Equal(t, diag.ApplyRestart, credential.Apply)
	assert.Equal(t, "api_key", modelField(t, c, diag.KeyModelCredentialKind).Effective.Value.Str)
}

func TestTheObservedImportIsTheOneThatActuallyRan(t *testing.T) {
	c := connectedDeveloperRuntime(t, "")

	state := modelField(t, c, diag.KeyModelImport)
	assert.Equal(t, "enabled", state.Configured.Value.Str, "the setting is what was asked for")
	assert.Equal(t, string(diag.ImportLoaded), state.Effective.Value.Str, "the state is what happened")
	assert.Equal(t, diag.ApplyRestart, state.Apply, "the import is read once, while the process starts")

	models := modelField(t, c, diag.KeyModelImportModels)
	require.Equal(t, diag.StatePresent, models.Effective.State)
	assert.Equal(t, int64(1), models.Effective.Value.Int,
		"the one catalog model the imported credential unlocks")
}

func TestASwitchedOffImportSaysSoAndCountsNothing(t *testing.T) {
	c := connectedDeveloperRuntime(t, "opencode:\n  enabled: false\n")

	state := modelField(t, c, diag.KeyModelImport)
	assert.Equal(t, "disabled", state.Configured.Value.Str)
	assert.Equal(t, string(diag.ImportDisabled), state.Effective.Value.Str)

	models := modelField(t, c, diag.KeyModelImportModels)
	assert.Equal(t, diag.StateNotApplicable, models.Effective.State,
		"an import that never happened has no count to be missing")

	// The provider catalog is read whether the import runs or not.
	assert.Equal(t, []string{"acme"}, modelField(t, c, diag.KeyModelConnected).Effective.Value.List)
}

func TestTheProviderAndImportViewCarryNoCredentialOfEither(t *testing.T) {
	c := connectedDeveloperRuntime(t, "")

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryModel)
	require.NoError(t, err)
	rendered := snapshot.JSON()

	for _, secret := range []string{
		catalogKeySentinel, importKeySentinel, catalogHostSentinel, "config-file-secret",
	} {
		assert.NotContains(t, rendered, secret,
			"the store and the import are counted and named, never quoted")
	}
	assert.Contains(t, rendered, "acme", "the provider's own id is a catalog fact")
	assert.Contains(t, rendered, `"loaded"`, "and the import says it ran")
}
