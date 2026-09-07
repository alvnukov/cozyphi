package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The strings a `web:` section carries all name something about the user's
// network rather than about this run, so each one is a sentinel: a host they
// can reach, the address their search goes through, the id of their own
// search engine, the agent every host would see, the variable holding the key
// and the key itself.
const (
	webCfgHost      = "wiki.headless-sentinel.example"
	webCfgDenied    = "metadata.headless-sentinel.example"
	webCfgSearchURL = "https://search.headless-sentinel.example/q"
	webCfgCSEURL    = "https://cse.headless-sentinel.example/v1"
	webCfgCSEID     = "headless-cse-id-sentinel"
	webCfgAgent     = "cozyphi-headless-sentinel/1.0"
	webCfgKeyEnv    = "COZYPHI_TEST_HEADLESS_CSE_SENTINEL"
	webCfgKey       = "AIzaSy-headless-key-sentinel"
	webCfgCacheName = "headless-web-cache-sentinel"
	webCfgAllow     = `^(.*\.)?headless-sentinel\.example$`
)

// webSentinels is everything the answer is checked against — including the
// bare word, so a partial quotation is caught as well as a whole one.
func webSentinels(cacheDir string) []string {
	return []string{
		webCfgHost, webCfgDenied, webCfgSearchURL, webCfgCSEURL, webCfgCSEID,
		webCfgAgent, webCfgKeyEnv, webCfgKey, webCfgAllow, cacheDir, "sentinel",
	}
}

// headlessWebRun makes one developer-mode round that asks the harness the
// given question over a run whose web section is fully written out. The cache
// directory is named and left uncreated, so a run that touched it can be told
// from one that only read the policy.
func headlessWebRun(t *testing.T, arguments string) (snapshot, cacheDir string) {
	t.Helper()
	t.Setenv(webCfgKeyEnv, webCfgKey)
	cacheDir = filepath.Join(t.TempDir(), webCfgCacheName)

	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", arguments)
		}
		return doneText()
	}, `web:
  enabled: true
  cache_dir: `+cacheDir+`
  max_source_bytes: 500000
  timeout_seconds: 20
  max_redirects: 2
  allowed_schemes:
    - https
  allowed_hosts:
    - `+webCfgHost+`
  denied_hosts:
    - `+webCfgDenied+`
  accepted_content_types:
    - text/html
    - text/plain
  user_agent: `+webCfgAgent+`
  search_provider: google_cse
  search_url: `+webCfgSearchURL+`
  max_search_results: 7
  google_cse_url: `+webCfgCSEURL+`
  google_cse_id: `+webCfgCSEID+`
  google_api_key_env: `+webCfgKeyEnv+`
  quarantine: reader
  allow:
    - `+webCfgAllow+`
`)
	require.NoDirExists(t, cacheDir, "the section named the directory and nothing has created it")

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what may you reach", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})
	require.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs, "the harness call must have produced a tool result")
	return outputs[0], cacheDir
}

// The headless run reports what it may reach the same way the TUI does: what
// the section asked for, what the engine was built with, and what a call made
// now would run under — all of it counted rather than quoted.
func TestTheHeadlessRunReportsWhatItMayReachOnTheInternet(t *testing.T) {
	snapshot, cacheDir := headlessWebRun(t, `{"action":"snapshot","category":"integrations"}`)
	fields := harnessFields(t, snapshot)

	state := fields[diag.KeyWebState]
	assert.True(t, state.Configured.Value.Bool, "the section switched network access on")
	assert.Equal(t, string(diag.WebReady), state.Effective.Value.String,
		"an enabled policy with somewhere to cache carries a web tool")

	assert.Equal(t, "reader", fields[diag.KeyWebQuarantine].Configured.Value.String)
	assert.True(t, fields[diag.KeyWebQuarantine].Effective.Value.Bool)
	assert.Equal(t, string(diag.WebCacheCustom), fields[diag.KeyWebCache].Configured.Value.String)
	assert.Equal(t, int64(1), fields[diag.KeyWebSchemes].Effective.Value.Int)
	assert.Equal(t, []string{"allowed=1", "denied=1", "policy=" + string(diag.WebHostsAllowList)},
		fields[diag.KeyWebHosts].Effective.Value.List)
	assert.Equal(t, []string{
		"max_source_bytes=500000",
		"timeout_seconds=20",
		"max_redirects=2",
		"content_types=2",
		"user_agent=custom",
	}, fields[diag.KeyWebFetch].Effective.Value.List)
	assert.Equal(t, []string{
		"provider=google_cse",
		"max_results=7",
		"search_url=set",
		"google_cse_url=set",
		"google_cse_id=set",
	}, fields[diag.KeyWebSearch].Effective.Value.List)
	assert.True(t, fields[diag.KeyWebCredential].Configured.Value.Bool)
	assert.True(t, fields[diag.KeyWebCredential].Effective.Value.Bool,
		"the variable the section names holds a key on this machine")

	for _, secret := range webSentinels(cacheDir) {
		assert.NotContains(t, snapshot, secret, "a web rule is counted or reported present, never quoted")
	}
	assert.NotContains(t, snapshot, "test-key", "and the provider credential never reaches an observation")
	assert.NoDirExists(t, cacheDir,
		"asking what may be reached does not open the cache a fetch would write to")
}

// The egress allow-list is a permission rule and is reported with the others:
// it is compiled into the same boundary and matched against the host a call
// would reach, so a reader asking what this run may talk to finds it beside
// bash.allow and mcp.allow.
func TestTheHeadlessRunCountsItsEgressRulesBesideTheOtherPermissions(t *testing.T) {
	snapshot, cacheDir := headlessWebRun(t, `{"action":"snapshot","category":"permissions"}`)
	fields := harnessFields(t, snapshot)

	allow := fields[diag.KeyPermissionWebAllow]
	assert.Equal(t, int64(1), allow.Configured.Value.Int)
	assert.Equal(t, int64(1), allow.Effective.Value.Int, "the rule is in the boundary this run judges on")
	assert.Empty(t, allow.Configured.Value.List, "there is nowhere for a host to be written")

	for _, secret := range webSentinels(cacheDir) {
		assert.NotContains(t, snapshot, secret)
	}
}

// The narrow question is where a leak would hide, because it is the one a
// reader asks when the whole category told them too little.
func TestExplainingOneWebFieldQuotesNoMoreThanTheSnapshotDoes(t *testing.T) {
	snapshot, cacheDir := headlessWebRun(t,
		`{"action":"explain","category":"integrations","key":"`+diag.KeyWebHosts+`"}`)

	assert.Contains(t, snapshot, `"key":"`+diag.KeyWebHosts+`"`)
	assert.Contains(t, snapshot, `"policy=`+string(diag.WebHostsAllowList)+`"`)
	for _, secret := range webSentinels(cacheDir) {
		assert.NotContains(t, snapshot, secret)
	}
	assert.NoDirExists(t, cacheDir)
}

// Listing what can be observed must not observe anything, and the headless
// catalog must declare the same web keys the TUI does — it is one contract,
// answered by one registry, at two surfaces.
func TestTheIntegrationCatalogDeclaresTheWebKeysToo(t *testing.T) {
	catalog, cacheDir := headlessWebRun(t, `{"action":"catalog"}`)

	for _, key := range []string{
		diag.KeyWebState, diag.KeyWebQuarantine, diag.KeyWebCache, diag.KeyWebSchemes,
		diag.KeyWebHosts, diag.KeyWebFetch, diag.KeyWebSearch, diag.KeyWebCredential,
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}
	assert.Contains(t, catalog, `"`+diag.KeyPermissionWebAllow+`"`)

	// A per-host key is the tempting shape and deliberately not offered:
	// answering it would put a host in the catalog itself.
	assert.NotContains(t, catalog, "web.egress.host.")
	for _, secret := range webSentinels(cacheDir) {
		assert.NotContains(t, catalog, secret)
	}
	assert.NoDirExists(t, cacheDir)
}

// A run with no `web:` section has to say that network access is off rather
// than report rules that permit everything: the two read the same in an empty
// answer and are fixed in different places.
func TestAHeadlessRunWithNoWebSectionReportsNetworkAccessOff(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return doneText()
	})

	require.Equal(t, ExitOK, runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what may you reach", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	}))
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	fields := harnessFields(t, outputs[0])

	assert.False(t, fields[diag.KeyWebState].Configured.Value.Bool, "the opt-in was never taken")
	assert.Equal(t, string(diag.WebOff), fields[diag.KeyWebState].Effective.Value.String)
	for _, key := range []string{diag.KeyWebHosts, diag.KeyWebFetch, diag.KeyWebSearch} {
		assert.Equal(t, "not_applicable", fields[key].Effective.State,
			"no fetch happens at all, so no bound applies: %s", key)
	}
	assert.NotContains(t, strings.ToLower(outputs[0]), "sentinel")
}
