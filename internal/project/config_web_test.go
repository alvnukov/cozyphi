package project

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebSectionDecodesIntoBothLayers: one `web:` block in the file feeds the
// library policy the tool runs under and the permission policy's host
// allow-list, because a user reasons about web access as one thing.
func TestWebSectionDecodesIntoBothLayers(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
web:
  enabled: true
  max_source_bytes: 500000
  timeout_seconds: 20
  allowed_schemes: https
  denied_hosts:
    - metadata.google.internal
  search_provider: google_cse
  google_cse_id: cse-123
  google_api_key_env: MY_CSE_KEY
  quarantine: reader
  allow:
    - ^(.*\.)?golang\.org$
`)

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	assert.True(t, cfg.Web.Enabled())
	assert.Equal(t, int64(500000), cfg.Web.Policy.MaxSourceBytes)
	assert.Equal(t, 20, cfg.Web.Policy.TimeoutSeconds)
	assert.Equal(t, []string{"https"}, cfg.Web.Policy.AllowedSchemes,
		"a scalar must read as a one-element list")
	assert.Equal(t, []string{"metadata.google.internal"}, cfg.Web.Policy.DeniedHosts)
	assert.Equal(t, "google_cse", cfg.Web.Policy.SearchProvider)
	assert.Equal(t, "cse-123", cfg.Web.Policy.GoogleCSEID)
	assert.Equal(t, "MY_CSE_KEY", cfg.Web.Policy.GoogleAPIKeyEnv)
	assert.Equal(t, WebQuarantineReader, cfg.Web.Quarantine)
	assert.Equal(t, []string{`^(.*\.)?golang\.org$`}, cfg.Permissions.WebAllow)
	assert.False(t, cfg.Permissions.WebDisabled)
}

// TestMissingWebSectionLeavesWebOff: the tool is opt-in, so an untouched
// config gets no network surface at all.
func TestMissingWebSectionLeavesWebOff(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	assert.False(t, cfg.Web.Enabled())
	assert.True(t, cfg.Permissions.WebDisabled)
	assert.Equal(t, WebQuarantineReader, cfg.Web.Quarantine, "the default must be the strict mode")
}

// TestWebCacheDirDefaultsToTheGlobalLayout: the fetched-page store belongs
// beside the other ~/.cozyphi state, not in the repository.
func TestWebCacheDirDefaultsToTheGlobalLayout(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n")

	require.NoError(t, p.LoadConfig())

	assert.Equal(t, p.Global().WebCacheDir(), p.Config().Web.Policy.CacheDir)
}

// TestWebCacheDirFromTheFileWins: a user who points the cache somewhere else
// keeps it.
func TestWebCacheDirFromTheFileWins(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p,
		"models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n  cache_dir: /var/tmp/pages\n")

	require.NoError(t, p.LoadConfig())

	assert.Equal(t, "/var/tmp/pages", p.Config().Web.Policy.CacheDir)
}

// TestQuarantineOffDecodesAsDataWithAWarning: the legacy mode still loads
// — an old config must not fail — but it no longer authorizes unchecked
// delivery, and the loader says so.
func TestQuarantineOffDecodesAsDataWithAWarning(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n  quarantine: off\n")

	require.NoError(t, p.LoadConfig())

	assert.Equal(t, WebQuarantineOff, p.Config().Web.Quarantine, "the mode is kept as data")

	var found string
	for _, w := range p.Config().Warnings() {
		if strings.Contains(w, "web.quarantine") {
			found = w
		}
	}
	require.NotEmpty(t, found, "decoding off silently would read as still working")
	assert.Contains(t, found, "no longer authorizes unchecked delivery")
	assert.Contains(t, found, "doc/web.md", "the warning must point at the setup documentation")
}

// TestEnabledTrueCarriesNoWarningAndNoReadiness: web.enabled: true is the
// tool's opt-in, nothing more — it loads without a warning and no field in
// the resolved config claims protected readiness.
func TestEnabledTrueCarriesNoWarningAndNoReadiness(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n")

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	assert.True(t, cfg.Web.Enabled(), "the opt-in still registers the tool")
	for _, w := range cfg.Warnings() {
		assert.NotContains(t, w, "web.quarantine", "an untouched quarantine setting needs no warning")
	}
}

// TestUnknownQuarantineModeFailsTheLoad: guessing which mode the user meant
// could silently pick the laxer one.
func TestUnknownQuarantineModeFailsTheLoad(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "web:\n  quarantine: maybe\n")

	err := p.LoadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "web.quarantine")
}

// TestGoogleAPIKeyLiteralIsDroppedWithAWarning is the config half of the
// egress rule: the CSE key travels in a query string, so it must live in the
// environment and never in a file the repo or a backup can carry.
func TestGoogleAPIKeyLiteralIsDroppedWithAWarning(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
web:
  enabled: true
  search_provider: google_cse
  google_api_key: AIza-literal-secret
  google_api_key_env: MY_CSE_KEY
`)

	require.NoError(t, p.LoadConfig())

	cfg := p.Config()
	assert.Empty(t, cfg.Web.Policy.GoogleAPIKey, "a literal key must not survive the loader")
	assert.Equal(t, "MY_CSE_KEY", cfg.Web.Policy.GoogleAPIKeyEnv)

	var found string
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, "web.google_api_key") {
			found = w
		}
	}
	require.NotEmpty(t, found, "dropping the key silently would look like a working config")
	assert.Contains(t, found, "google_api_key_env", "the warning must name the supported way")
	assert.NotContains(t, found, "AIza-literal-secret", "the warning must not repeat the secret")
}

// TestParseWebQuarantineNormalizes covers the parser directly, including the
// empty string a rewritten config can leave behind.
func TestParseWebQuarantineNormalizes(t *testing.T) {
	for _, in := range []string{"", "reader", " READER "} {
		mode, err := ParseWebQuarantine(in)
		require.NoError(t, err, in)
		assert.Equal(t, WebQuarantineReader, mode, in)
	}
	mode, err := ParseWebQuarantine("Off")
	require.NoError(t, err)
	assert.Equal(t, WebQuarantineOff, mode)

	_, err = ParseWebQuarantine("none")
	require.Error(t, err)
}

// TestSectionWithoutEnabledStaysOff: writing any other web key — an
// allow-list, a provider — configures the tool but does not switch it on.
// While protected web is not ready an enabled tool only refuses, so opting
// in must be a deliberate `enabled: true`, not a side effect; the default
// flips to on once the protected web model binding lands.
func TestSectionWithoutEnabledStaysOff(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p,
		"models:\n  - name: m\n    api_key: k\nweb:\n  allow:\n    - ^example\\.com$\n")

	require.NoError(t, p.LoadConfig())

	assert.False(t, p.Config().Web.Enabled())
	assert.True(t, p.Config().Permissions.WebDisabled)
}

// TestExplicitDisableWins keeps the off switch reachable for a config that
// already carries a web block.
func TestExplicitDisableWins(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p,
		"models:\n  - name: m\n    api_key: k\nweb:\n  enabled: false\n  search_provider: google_cse\n")

	require.NoError(t, p.LoadConfig())

	assert.False(t, p.Config().Web.Enabled())
	assert.True(t, p.Config().Permissions.WebDisabled)
}
