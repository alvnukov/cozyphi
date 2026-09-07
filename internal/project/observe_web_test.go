package project

import (
	"encoding/json"
	"path/filepath"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The sentinels below are the web text a user writes: hosts on their own
// network, the address their search goes through, the id of their own search
// engine, the agent string every host they reach will see, and the key
// itself. Every one of them says something about the user rather than about
// this session, so none may appear in an observation.
const (
	allowedHostSentinel = "wiki.internal-sentinel.example"
	deniedHostSentinel  = "metadata.internal-sentinel.example"
	searchURLSentinel   = "https://search.internal-sentinel.example/q"
	cseURLSentinel      = "https://cse.internal-sentinel.example/v1"
	cseIDSentinel       = "cse-id-sentinel"
	agentSentinel       = "cozyphi-sentinel/1.0"
	keyEnvSentinel      = "MY_CSE_KEY_SENTINEL"
	keyLiteralSentinel  = "AIzaSy-literal-sentinel"
	cacheDirSentinel    = "web-cache-sentinel"
)

// sentinelWebConfig writes a `web:` section whose every string is one of the
// sentinels above and loads it the way a session does, so what is observed is
// what the loader actually produced rather than a struct a test filled in.
func sentinelWebConfig(t *testing.T) (*Project, WebConfig) {
	t.Helper()
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
web:
  enabled: true
  cache_dir: `+filepath.Join(t.TempDir(), cacheDirSentinel)+`
  max_source_bytes: 500000
  timeout_seconds: 20
  max_redirects: 2
  allowed_schemes:
    - https
  allowed_hosts:
    - `+allowedHostSentinel+`
  denied_hosts:
    - `+deniedHostSentinel+`
  accepted_content_types:
    - text/html
    - text/plain
  user_agent: `+agentSentinel+`
  search_provider: google_cse
  search_url: `+searchURLSentinel+`
  max_search_results: 7
  google_cse_url: `+cseURLSentinel+`
  google_cse_id: `+cseIDSentinel+`
  google_api_key_env: `+keyEnvSentinel+`
  quarantine: reader
`)
	require.NoError(t, p.LoadConfig())
	return p, p.Config().Web
}

// observed renders the facts the way the harness would, so an assertion about
// what leaves is made against everything that leaves rather than member by
// member.
func observed(t *testing.T, facts diag.WebConfigFacts) string {
	t.Helper()
	out, err := json.Marshal(facts)
	require.NoError(t, err)
	return string(out)
}

// Every list is counted, every address and id is a presence, and the one
// string that survives is out of the library's own vocabulary of providers.
func TestObservingTheWebSectionCountsItsRulesAndQuotesNone(t *testing.T) {
	p, cfg := sentinelWebConfig(t)

	facts := ObserveWeb(cfg, p.Global().WebCacheDir())

	require.True(t, facts.Known)
	assert.Equal(t, "reader", facts.Quarantine)
	assert.Equal(t, diag.WebCacheCustom, facts.Cache, "the file pointed the cache somewhere of its own")

	policy := facts.Policy
	assert.True(t, policy.Enabled)
	assert.True(t, policy.CacheSet)
	assert.Equal(t, 1, policy.Schemes)
	assert.True(t, policy.SchemesConfigured)
	assert.Equal(t, 1, policy.AllowedHosts)
	assert.Equal(t, 1, policy.DeniedHosts)
	assert.Equal(t, 2, policy.ContentTypes)
	assert.True(t, policy.ContentTypesConfigured)
	assert.Equal(t, int64(500000), policy.MaxSourceBytes)
	assert.Equal(t, 20, policy.TimeoutSeconds)
	assert.Equal(t, 2, policy.MaxRedirects)
	assert.True(t, policy.CustomUserAgent)
	assert.Equal(t, "google_cse", policy.SearchProvider,
		"which provider is a name the library defines, not an address the user wrote")
	assert.Equal(t, 7, policy.MaxSearchResults)
	assert.True(t, policy.SearchEndpoint)
	assert.True(t, policy.CSEEndpoint)
	assert.True(t, policy.CSEID)
	assert.True(t, policy.CredentialEnv)

	rendered := observed(t, facts)
	for _, secret := range []string{
		allowedHostSentinel, deniedHostSentinel, searchURLSentinel, cseURLSentinel,
		cseIDSentinel, agentSentinel, keyEnvSentinel, cacheDirSentinel, "sentinel",
	} {
		assert.NotContains(t, rendered, secret, "a web rule is counted or reported present, never quoted")
	}
}

// A key literal is refused by the loader and forced out of the policy, so it
// is not a setting any more and there is nothing for an observation to report
// about it — least of all its presence, which would be a presence of a key
// that will never be used.
func TestARefusedKeyLiteralIsNotObservedAsACredential(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
web:
  enabled: true
  google_api_key: `+keyLiteralSentinel+`
`)
	require.NoError(t, p.LoadConfig())

	facts := ObserveWeb(p.Config().Web, p.Global().WebCacheDir())

	assert.False(t, facts.Policy.CredentialEnv,
		"the file named no variable, and the literal it did name never reached the policy")
	assert.NotContains(t, observed(t, facts), keyLiteralSentinel)
}

// A list nobody wrote and a list somebody emptied are two different answers:
// the first leaves the library's own set in force and the second is a
// boundary that admits nothing. Counting both as zero would merge them.
func TestAnUnwrittenWebListIsToldApartFromAnEmptyOne(t *testing.T) {
	unwritten := ObserveWebPolicy(cozyconfig.WebPolicy{})
	assert.False(t, unwritten.SchemesConfigured)
	assert.Zero(t, unwritten.Schemes)
	assert.False(t, unwritten.ContentTypesConfigured)

	emptied := ObserveWebPolicy(cozyconfig.WebPolicy{
		AllowedSchemes:       []string{},
		AcceptedContentTypes: []string{},
	})
	assert.True(t, emptied.SchemesConfigured, "a list is written even when what it holds is nothing")
	assert.Zero(t, emptied.Schemes)
	assert.True(t, emptied.ContentTypesConfigured)

	// The same distinction as the loader produces it: a section with no
	// allowed_schemes key leaves the list unwritten.
	_, cfg := sentinelWebConfig(t)
	assert.True(t, ObserveWebPolicy(cfg.Policy).SchemesConfigured)

	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n")
	require.NoError(t, p.LoadConfig())
	assert.False(t, ObserveWebPolicy(p.Config().Web.Policy).SchemesConfigured)
}

// Where fetched documents land is a kind and never a path. The absent one is
// the case that matters most: it leaves the web tool out entirely, however
// enabled the policy is, and a reader looking at "enabled: true" needs the
// view to say so.
func TestTheWebCacheKindSaysWhereWithoutSayingWhere(t *testing.T) {
	root := t.TempDir()
	layout := filepath.Join(root, "web")

	assert.Equal(t, diag.WebCacheUnset, webCacheKind("", layout))
	assert.Equal(t, diag.WebCacheUnset, webCacheKind("   ", layout),
		"a directory of blanks is no directory, and the tool is left out on the same terms")
	assert.Equal(t, diag.WebCacheDefault, webCacheKind(layout, layout))
	assert.Equal(t, diag.WebCacheCustom, webCacheKind(filepath.Join(root, cacheDirSentinel), layout))

	// And through the loader, which fills an unset directory from the layout
	// before anything observes it.
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\nweb:\n  enabled: true\n")
	require.NoError(t, p.LoadConfig())
	assert.Equal(t, diag.WebCacheDefault,
		ObserveWeb(p.Config().Web, p.Global().WebCacheDir()).Cache)
}

// A configuration with no `web:` section at all is still an answer: the tool
// is opt-in, so the section resolves to network access switched off rather
// than to nothing being known.
func TestAWebSectionNobodyWroteIsObservedAsSwitchedOff(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(t, p, "models:\n  - name: m\n    api_key: k\n")
	require.NoError(t, p.LoadConfig())

	facts := ObserveWeb(p.Config().Web, p.Global().WebCacheDir())

	require.True(t, facts.Known, "the loader resolved the section; that it resolved to off is the answer")
	assert.False(t, facts.Policy.Enabled)
	assert.Equal(t, string(WebQuarantineReader), facts.Quarantine,
		"the strict mode stands even with nothing to quarantine")
}
