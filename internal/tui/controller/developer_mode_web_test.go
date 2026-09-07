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

// The sentinels are what a real web block carries and what a real one must
// never leak: hosts on the user's own network, the address their search goes
// through, the id of their own search engine, the agent string every host
// they reach would see, and the directory their fetched pages land in.
const (
	webHostSentinel      = "wiki.sentinel-internal.example"
	webDeniedSentinel    = "metadata.sentinel-internal.example"
	webSearchSentinel    = "https://search.sentinel-internal.example/q"
	webCSEURLSentinel    = "https://cse.sentinel-internal.example/v1"
	webCSEIDSentinel     = "cse-id-sentinel"
	webAgentSentinel     = "cozyphi-sentinel/1.0"
	webKeyEnvSentinel    = "COZYPHI_TEST_CSE_SENTINEL"
	webKeySentinel       = "AIzaSy-live-key-sentinel"
	webCacheDirSentinel  = "web-cache-sentinel"
	webEgressAllowRegexp = `^(.*\.)?sentinel-internal\.example$`
)

// developerWebRuntime starts a granted developer session over a workspace
// whose web block is fully written out, so the policy reported is one the
// loader actually resolved rather than a fixture's idea of one. The cache
// directory is named and deliberately not created: whether asking about web
// access creates it is one of the things under test.
func developerWebRuntime(t *testing.T) (c *Controller, cacheDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	t.Setenv(webKeyEnvSentinel, webKeySentinel)
	cwd := t.TempDir()
	cacheDir = filepath.Join(t.TempDir(), webCacheDirSentinel)

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	body := twoModelConfig("alpha") + `web:
  enabled: true
  cache_dir: ` + cacheDir + `
  max_source_bytes: 500000
  timeout_seconds: 20
  max_redirects: 2
  allowed_schemes:
    - https
  allowed_hosts:
    - ` + webHostSentinel + `
  denied_hosts:
    - ` + webDeniedSentinel + `
  accepted_content_types:
    - text/html
    - text/plain
  user_agent: ` + webAgentSentinel + `
  search_provider: google_cse
  search_url: ` + webSearchSentinel + `
  max_search_results: 7
  google_cse_url: ` + webCSEURLSentinel + `
  google_cse_id: ` + webCSEIDSentinel + `
  google_api_key_env: ` + webKeyEnvSentinel + `
  quarantine: reader
  allow:
    - ` + webEgressAllowRegexp + `
`
	require.NoError(t, os.WriteFile(configFile, []byte(body), 0o600))

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err = rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, cacheDir
}

func webField(t *testing.T, c *Controller, category diag.Category, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), category, key)
	require.NoError(t, err)
	return explained.Field
}

// What a session may reach on the open internet is the question this category
// exists for, and it is answered on all three layers: what the file asked
// for, what the engine was built with, and what a call made now would run
// under.
func TestTheHarnessReportsWhatThisSessionMayReachOnTheInternet(t *testing.T) {
	c, _ := developerWebRuntime(t)

	state := webField(t, c, diag.CategoryIntegrations, diag.KeyWebState)
	assert.True(t, state.Configured.Value.Bool, "the file switched web access on")
	assert.True(t, state.Loaded.Value.Bool)
	assert.Equal(t, string(diag.WebReady), state.Effective.Value.Str,
		"an enabled policy with somewhere to cache carries a web tool")

	quarantine := webField(t, c, diag.CategoryIntegrations, diag.KeyWebQuarantine)
	assert.Equal(t, "reader", quarantine.Configured.Value.Str)
	assert.True(t, quarantine.Effective.Value.Bool, "a fetched page reaches this session through the reader")

	assert.Equal(t, string(diag.WebCacheCustom),
		webField(t, c, diag.CategoryIntegrations, diag.KeyWebCache).Configured.Value.Str)
	assert.Equal(t, int64(1),
		webField(t, c, diag.CategoryIntegrations, diag.KeyWebSchemes).Effective.Value.Int)
	assert.Equal(t, []string{"allowed=1", "denied=1", "policy=allow_list"},
		webField(t, c, diag.CategoryIntegrations, diag.KeyWebHosts).Effective.Value.List)
	assert.Equal(t, []string{
		"max_source_bytes=500000",
		"timeout_seconds=20",
		"max_redirects=2",
		"content_types=2",
		"user_agent=custom",
	}, webField(t, c, diag.CategoryIntegrations, diag.KeyWebFetch).Effective.Value.List)
	assert.Equal(t, []string{
		"provider=google_cse",
		"max_results=7",
		"search_url=set",
		"google_cse_url=set",
		"google_cse_id=set",
	}, webField(t, c, diag.CategoryIntegrations, diag.KeyWebSearch).Effective.Value.List)

	credential := webField(t, c, diag.CategoryIntegrations, diag.KeyWebCredential)
	assert.True(t, credential.Configured.Value.Bool, "the file names a variable to read the key from")
	assert.True(t, credential.Effective.Value.Bool, "and on this machine it holds one")

	// The egress allow-list is a permission rule and lives with the others:
	// it is compiled into the same boundary and matched against the host.
	assert.Equal(t, int64(1),
		webField(t, c, diag.CategoryPermissions, diag.KeyPermissionWebAllow).Effective.Value.Int)
}

// Every string a web block carries names something about the user rather than
// about this session. The counts and presences above are the whole answer, so
// this is the test that notices when a literal starts traveling with them.
func TestAskingAboutWebNamesNoHostSchemeAddressOrKey(t *testing.T) {
	c, _ := developerWebRuntime(t)

	secrets := []string{
		webHostSentinel, webDeniedSentinel, webSearchSentinel, webCSEURLSentinel,
		webCSEIDSentinel, webAgentSentinel, webKeyEnvSentinel, webKeySentinel,
		webCacheDirSentinel, webEgressAllowRegexp, "sentinel",
	}

	// The narrow answer, the whole category and the overview: a literal that
	// is withheld from one and carried by another is withheld from none.
	snapshots := []func() []diag.Field{
		func() []diag.Field {
			snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
			require.NoError(t, err)
			require.Len(t, snapshot.Categories, 1)
			return snapshot.Categories[0].Fields
		},
		func() []diag.Field {
			snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPermissions)
			require.NoError(t, err)
			require.Len(t, snapshot.Categories, 1)
			return snapshot.Categories[0].Fields
		},
		func() []diag.Field {
			snapshot, err := c.diagnostics.Snapshot(t.Context(), "")
			require.NoError(t, err)
			var all []diag.Field
			for _, category := range snapshot.Categories {
				all = append(all, category.Fields...)
			}
			return all
		},
	}

	for _, fields := range snapshots {
		for _, field := range fields() {
			for _, observation := range []diag.Observation{field.Configured, field.Loaded, field.Effective} {
				for _, secret := range secrets {
					assert.NotContains(t, observation.Value.Str, secret, field.Key)
					assert.NotContains(t, observation.Source.Ref, secret, field.Key)
					for _, item := range observation.Value.List {
						assert.NotContains(t, item, secret, field.Key)
					}
				}
			}
		}
	}
}

// The question must be free to ask. If reading the category opened a
// connection, a developer looking at the harness would reach a host by
// looking at a list — and a session that had reached nothing would be tainted
// by the answer about it.
func TestAskingAboutWebOpensNoConnectionAndTouchesNoCache(t *testing.T) {
	c, cacheDir := developerWebRuntime(t)
	require.NoDirExists(t, cacheDir, "the fixture named the directory and left it uncreated")

	before := webField(t, c, diag.CategoryIntegrations, diag.KeyWebState)
	for range 3 {
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
		require.NoError(t, err)
		_, err = c.diagnostics.Snapshot(t.Context(), "")
		require.NoError(t, err)
		require.NotEmpty(t, c.diagnostics.Catalog().Categories)
		for _, key := range []string{diag.KeyWebState, diag.KeyWebHosts, diag.KeyWebCredential} {
			webField(t, c, diag.CategoryIntegrations, key)
		}
	}

	assert.NoDirExists(t, cacheDir,
		"three rounds of questions and the cache the tool would write to still does not exist")
	after := webField(t, c, diag.CategoryIntegrations, diag.KeyWebState)
	assert.Equal(t, before.Effective, after.Effective,
		"and the answer is the same one, so nothing about the session was consumed by asking")
}

// The tool is opt-in, and a session without it has to say so plainly: a
// developer who sees empty egress rules must be able to tell "no rules" from
// "no web at all".
func TestASessionWithNoWebToolSaysSoRatherThanReportingEmptyRules(t *testing.T) {
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

	state := webField(t, c, diag.CategoryIntegrations, diag.KeyWebState)
	assert.False(t, state.Configured.Value.Bool, "no web: section is the opt-in never taken")
	assert.Equal(t, string(diag.WebOff), state.Effective.Value.Str)

	for _, key := range []string{diag.KeyWebHosts, diag.KeyWebFetch, diag.KeyWebSearch} {
		field := webField(t, c, diag.CategoryIntegrations, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State,
			"an empty answer would read as rules that permit everything: %s", key)
		assert.NotEmpty(t, field.Effective.Source.Ref, "and the field says why: %s", key)
	}
}
