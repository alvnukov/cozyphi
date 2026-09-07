package agent

import (
	"encoding/json"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
)

// The sentinels are the strings a user's own web block carries: a host on
// their network, the address their search goes through, the id of their own
// search engine, the name of the variable holding the key and the key itself.
// None of them describes this session, so none may leave the engine.
const (
	engineHostSentinel   = "wiki.internal-sentinel.example"
	engineSearchSentinel = "https://search.internal-sentinel.example/q"
	engineCSESentinel    = "cse-id-sentinel"
	engineKeyEnvSentinel = "COZYPHI_TEST_CSE_KEY_SENTINEL"
	engineKeySentinel    = "AIzaSy-live-key-sentinel"
)

// sentinelWebPolicy is a policy with something in every list and every
// address, so an observation of it has every chance to leak one.
func sentinelWebPolicy(t *testing.T) cozyconfig.WebPolicy {
	t.Helper()
	on := true
	return cozyconfig.WebPolicy{
		Enabled:              &on,
		CacheDir:             t.TempDir(),
		MaxSourceBytes:       500000,
		TimeoutSeconds:       20,
		MaxRedirects:         2,
		AllowedSchemes:       []string{"https"},
		AllowedHosts:         []string{engineHostSentinel},
		DeniedHosts:          []string{"metadata." + engineHostSentinel},
		AcceptedContentTypes: []string{"text/html"},
		UserAgent:            "cozyphi-sentinel/1.0",
		SearchProvider:       "google_cse",
		SearchURL:            engineSearchSentinel,
		MaxSearchResults:     7,
		GoogleCSEID:          engineCSESentinel,
		GoogleAPIKeyEnv:      engineKeyEnvSentinel,
	}
}

func webEngine(t *testing.T, web WebOptions) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
		Web:         web,
	})
	require.NoError(t, err)
	return engine
}

func renderedWeb(t *testing.T, facts diag.WebRuntimeFacts) string {
	t.Helper()
	out, err := json.Marshal(facts)
	require.NoError(t, err)
	return string(out)
}

// The engine answers for the policy it was built with — which is the one that
// would bound a call made now, and not necessarily the one on disk — and it
// answers in counts and presences.
func TestTheEngineReportsTheWebPolicyItWasBuiltWith(t *testing.T) {
	engine := webEngine(t, WebOptions{Policy: sentinelWebPolicy(t), Quarantine: true})

	facts := engine.WebObservation()

	require.True(t, facts.Known)
	assert.True(t, facts.ToolPresent, "an enabled policy with somewhere to cache carries a web tool")
	assert.True(t, facts.Quarantine)
	assert.True(t, facts.Policy.Enabled)
	assert.True(t, facts.Policy.CacheSet)
	assert.Equal(t, 1, facts.Policy.AllowedHosts)
	assert.Equal(t, 1, facts.Policy.DeniedHosts)
	assert.Equal(t, 1, facts.Policy.Schemes)
	assert.True(t, facts.Policy.SearchEndpoint)
	assert.True(t, facts.Policy.CSEID)
	assert.True(t, facts.Policy.CredentialEnv)

	rendered := renderedWeb(t, facts)
	for _, secret := range []string{
		engineHostSentinel, engineSearchSentinel, engineCSESentinel, engineKeyEnvSentinel, "sentinel",
	} {
		assert.NotContains(t, rendered, secret, "an egress rule is counted or reported present, never quoted")
	}
}

// The two ways an engine ends up with no web tool are set in different places
// and read the same from outside, so the observation carries both facts and
// lets the view tell them apart.
func TestAnEngineWithoutAWebToolSaysWhichSettingLeftItOut(t *testing.T) {
	on := true
	off := false

	switchedOff := webEngine(t, WebOptions{
		Policy: cozyconfig.WebPolicy{Enabled: &off, CacheDir: t.TempDir()},
	}).WebObservation()
	assert.False(t, switchedOff.ToolPresent)
	assert.False(t, switchedOff.Policy.Enabled)
	assert.True(t, switchedOff.Policy.CacheSet, "the cache is set; it is the switch that is not")

	nowhereToCache := webEngine(t, WebOptions{
		Policy: cozyconfig.WebPolicy{Enabled: &on},
	}).WebObservation()
	assert.False(t, nowhereToCache.ToolPresent)
	assert.True(t, nowhereToCache.Policy.Enabled, "the switch is on; there is nowhere to put a page")
	assert.False(t, nowhereToCache.Policy.CacheSet)
}

// The credential is read the way websearch reads it and reported as a
// presence. The value never leaves, and neither does the name of the variable
// it was read from.
func TestTheWebCredentialIsAPresenceAndNeverTheKey(t *testing.T) {
	t.Setenv(engineKeyEnvSentinel, engineKeySentinel)
	held := webEngine(t, WebOptions{Policy: sentinelWebPolicy(t)}).WebObservation()
	assert.True(t, held.Credential)
	assert.NotContains(t, renderedWeb(t, held), engineKeySentinel)

	t.Setenv(engineKeyEnvSentinel, "   ")
	blank := webEngine(t, WebOptions{Policy: sentinelWebPolicy(t)}).WebObservation()
	assert.False(t, blank.Credential,
		"a variable holding blanks holds no key, and reporting one would send a reader looking for a "+
			"failure somewhere else")
	assert.True(t, blank.Policy.CredentialEnv, "the policy still names a variable, and that is a fact of its own")

	unnamed := sentinelWebPolicy(t)
	unnamed.GoogleAPIKeyEnv = ""
	none := webEngine(t, WebOptions{Policy: unnamed}).WebObservation()
	assert.False(t, none.Policy.CredentialEnv)
	assert.False(t, none.Credential, "nothing is read when nothing names a variable to read")
}

// Asking an engine about its web layer must not build a tool, reach a host or
// touch the cache — an observation that opened a connection would be the one
// question a read-only view may never answer.
func TestObservingWebLeavesTheEngineAndItsCacheAlone(t *testing.T) {
	policy := sentinelWebPolicy(t)
	engine := webEngine(t, WebOptions{Policy: policy, Quarantine: true})

	before := engine.WebObservation()
	for range 3 {
		assert.Equal(t, before, engine.WebObservation(),
			"the same question asked again is the same answer: nothing was built or consumed by asking")
	}
	assert.Equal(t, policy, engine.web.Policy, "and the policy the engine holds is the one it started with")

	var absent *Engine
	assert.Equal(t, diag.WebRuntimeFacts{}, absent.WebObservation(),
		"a surface with no engine yet reports unknown rather than a session that cannot reach the network")
}

// WebObservation is the engine's half; the loader's half comes from project.
// They are the same shape because it is the same question asked of two
// layers, and a view that could not compare them would not be able to say
// that a reload has not been picked up.
func TestTheEngineAndTheLoaderAnswerTheSameShape(t *testing.T) {
	policy := sentinelWebPolicy(t)
	engine := webEngine(t, WebOptions{Policy: policy, Quarantine: true})

	assert.Equal(t, project.ObserveWebPolicy(policy), engine.WebObservation().Policy)
}
