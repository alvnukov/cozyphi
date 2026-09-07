package diag_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveWebPolicy is a policy with every kind of answer in it at once: lists
// somebody wrote, bounds somebody set, a user agent, a search provider with
// both of its addresses and an id, and a credential named by a variable.
func liveWebPolicy() diag.WebPolicyFacts {
	return diag.WebPolicyFacts{
		Enabled:                true,
		CacheSet:               true,
		Schemes:                1,
		SchemesConfigured:      true,
		AllowedHosts:           2,
		DeniedHosts:            1,
		ContentTypes:           3,
		ContentTypesConfigured: true,
		MaxSourceBytes:         524288,
		TimeoutSeconds:         30,
		MaxRedirects:           3,
		CustomUserAgent:        true,
		SearchProvider:         "google",
		MaxSearchResults:       5,
		SearchEndpoint:         true,
		CSEEndpoint:            true,
		CSEID:                  true,
		CredentialEnv:          true,
	}
}

// liveWeb is a session whose web tool exists and could reach out: the file
// asked for it, the engine was built with it, and the reader stands in front
// of whatever it fetches.
func liveWeb() diag.WebState {
	policy := liveWebPolicy()
	return diag.WebState{
		Config: diag.WebConfigFacts{
			Known:      true,
			Policy:     policy,
			Quarantine: "reader",
			Cache:      diag.WebCacheCustom,
		},
		Runtime: diag.WebRuntimeFacts{
			Known:       true,
			Policy:      policy,
			ToolPresent: true,
			Quarantine:  true,
			Credential:  true,
		},
	}
}

func webFields(t *testing.T, state diag.WebState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{Web: func() diag.WebState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// webKeys is every field the web half of the category answers, which is also
// the list a reader has to be able to trust when nothing is wired.
var webKeys = []string{
	diag.KeyWebState,
	diag.KeyWebQuarantine,
	diag.KeyWebCache,
	diag.KeyWebSchemes,
	diag.KeyWebHosts,
	diag.KeyWebFetch,
	diag.KeyWebSearch,
	diag.KeyWebCredential,
}

// "This session has no web tool" is three different situations with three
// different fixes, and two of them are settings rather than faults: switched
// off, or switched on with nowhere to put what it would fetch.
func TestTheWebLifecycleSeparatesTheWaysThereCanBeNoWebTool(t *testing.T) {
	for _, tc := range []struct {
		name      string
		runtime   diag.WebRuntimeFacts
		effective diag.WebLifecycle
	}{
		{
			name:      "switched off",
			runtime:   diag.WebRuntimeFacts{Known: true},
			effective: diag.WebOff,
		},
		{
			name:      "on, with nowhere to cache what it fetches",
			runtime:   diag.WebRuntimeFacts{Known: true, Policy: diag.WebPolicyFacts{Enabled: true}},
			effective: diag.WebNoCacheDir,
		},
		{
			name:      "carrying a tool a call could reach out with",
			runtime:   liveWeb().Runtime,
			effective: diag.WebReady,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := diag.WebState{Config: liveWeb().Config, Runtime: tc.runtime}
			field := fieldByKey(t, webFields(t, state), diag.KeyWebState)
			assert.Equal(t, string(tc.effective), field.Effective.Value.Str)
			assert.True(t, field.Configured.Value.Bool, "the file asked for web access in every case")
			assert.Equal(t, diag.ApplyRestart, field.Apply,
				"the tool layer is built once, so a changed policy needs a new session")
			assert.Equal(t, diag.ScopeWorkspace, field.Scope)
		})
	}
}

// The file asking for web access and the engine having been built with it
// are two different facts, and the gap between them is what a reader is
// looking for after a reload that has not been picked up yet.
func TestTheConfiguredAndBuiltWebPolicyAreDifferentAnswers(t *testing.T) {
	state := liveWeb()
	state.Runtime.Policy.Enabled = false
	state.Runtime.ToolPresent = false

	field := fieldByKey(t, webFields(t, state), diag.KeyWebState)
	assert.True(t, field.Configured.Value.Bool, "the configuration on disk switches web access on")
	assert.False(t, field.Loaded.Value.Bool, "the engine in this session was built without it")
	assert.Equal(t, string(diag.WebOff), field.Effective.Value.Str)
	assert.Equal(t, diag.SourceConfigFile, field.Configured.Source.Kind)
	assert.Equal(t, diag.SourceSession, field.Loaded.Source.Kind)
	assert.Equal(t, diag.SourceSession, field.Effective.Source.Kind)
}

// A session with no web tool has answered the question every bound asks: no
// fetch happens at all, so nothing bounds one. Reporting the policy's numbers
// there would describe limits on a call that cannot be made.
func TestASessionWithNoWebToolReportsNoBoundsRatherThanZeroOnes(t *testing.T) {
	state := liveWeb()
	state.Runtime.ToolPresent = false
	fields := webFields(t, state)

	for _, key := range []string{
		diag.KeyWebQuarantine,
		diag.KeyWebSchemes,
		diag.KeyWebHosts,
		diag.KeyWebFetch,
		diag.KeyWebSearch,
		diag.KeyWebCredential,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.NotEmpty(t, field.Effective.Source.Ref, "and the field says why: %s", key)
		assert.Empty(t, field.Effective.Value.List, key)
		assert.NotEqual(t, diag.StateNotApplicable, field.Loaded.State,
			"what the engine was built with is still knowable and still worth saying: %s", key)
	}

	assert.Equal(t, diag.StatePresent, fieldByKey(t, fields, diag.KeyWebState).Effective.State,
		"the one thing still knowable is why there is no tool, and it is still said")
	assert.Equal(t, diag.StatePresent, fieldByKey(t, fields, diag.KeyWebCache).Effective.State,
		"and so is whether a cache directory is set, which is one of the two reasons")
}

// A list nobody wrote leaves the library's own in force. Counting it as zero
// would read as a boundary that admits no scheme at all, which is the
// opposite of what is happening.
func TestAnUnwrittenSchemeListIsNotAnEmptyOne(t *testing.T) {
	written := fieldByKey(t, webFields(t, liveWeb()), diag.KeyWebSchemes)
	assert.Equal(t, diag.StatePresent, written.Configured.State)
	assert.Equal(t, int64(1), written.Configured.Value.Int)

	state := liveWeb()
	state.Config.Policy.SchemesConfigured = false
	state.Config.Policy.Schemes = 0
	state.Runtime.Policy.SchemesConfigured = false
	state.Runtime.Policy.Schemes = 0

	unwritten := fieldByKey(t, webFields(t, state), diag.KeyWebSchemes)
	assert.Equal(t, diag.StateUnset, unwritten.Configured.State,
		"the file named no scheme, so the library's own set decides")
	assert.Equal(t, diag.StateUnset, unwritten.Effective.State)
	assert.NotEmpty(t, unwritten.Configured.Source.Ref)

	// The same distinction on the fetch field, where it is one label among
	// several rather than a state of its own.
	emptied := liveWeb()
	emptied.Config.Policy.ContentTypes = 0
	emptied.Config.Policy.ContentTypesConfigured = true
	assert.Contains(t, fieldByKey(t, webFields(t, emptied), diag.KeyWebFetch).Configured.Value.List,
		"content_types=0", "a list somebody emptied is a real answer of zero")

	untouched := liveWeb()
	untouched.Config.Policy.ContentTypesConfigured = false
	untouched.Config.Policy.ContentTypes = 0
	assert.Contains(t, fieldByKey(t, webFields(t, untouched), diag.KeyWebFetch).Configured.Value.List,
		"content_types=default", "and a list nobody wrote is not")
}

// Which hosts the rules name is the one thing a reader might want and the one
// thing that cannot travel: both lists are written by the user and can name
// any machine on any network they can see.
func TestTheHostRulesTravelAsAShapeAndNeverAsAList(t *testing.T) {
	fields := webFields(t, liveWeb())
	hosts := fieldByKey(t, fields, diag.KeyWebHosts)

	assert.Equal(t, []string{"allowed=2", "denied=1", "policy=allow_list"}, hosts.Configured.Value.List)
	assert.Equal(t, hosts.Configured.Value.List, hosts.Effective.Value.List)

	open := liveWeb()
	open.Config.Policy.AllowedHosts = 0
	open.Runtime.Policy.AllowedHosts = 0
	assert.Equal(t, []string{"allowed=0", "denied=1", "policy=open"},
		fieldByKey(t, webFields(t, open), diag.KeyWebHosts).Configured.Value.List,
		"with no allow-list every host the deny list misses is reachable, and that is the answer")
}

// A bound that is not positive is not a bound of zero: it is the one the
// library substitutes at the point of use. Reporting `max_redirects=0` would
// read as "no redirect is followed", which is the opposite of the truth.
func TestANonPositiveWebBoundIsADefaultAndNotAZero(t *testing.T) {
	set := fieldByKey(t, webFields(t, liveWeb()), diag.KeyWebFetch)
	assert.Equal(t, []string{
		"max_source_bytes=524288",
		"timeout_seconds=30",
		"max_redirects=3",
		"content_types=3",
		"user_agent=custom",
	}, set.Configured.Value.List)

	unset := liveWeb()
	unset.Config.Policy.MaxSourceBytes = 0
	unset.Config.Policy.TimeoutSeconds = 0
	unset.Config.Policy.MaxRedirects = 0
	unset.Config.Policy.CustomUserAgent = false
	assert.Equal(t, []string{
		"max_source_bytes=default",
		"timeout_seconds=default",
		"max_redirects=default",
		"content_types=3",
		"user_agent=default",
	}, fieldByKey(t, webFields(t, unset), diag.KeyWebFetch).Configured.Value.List)
}

// Search says that it is configured without saying where it points or on
// whose behalf: a search URL can carry a token in its path or its query, and
// a custom-search id names the user's own engine.
func TestSearchTravelsAsPresenceAndNeverAsAnAddress(t *testing.T) {
	search := fieldByKey(t, webFields(t, liveWeb()), diag.KeyWebSearch)
	assert.Equal(t, []string{
		"provider=google",
		"max_results=5",
		"search_url=set",
		"google_cse_url=set",
		"google_cse_id=set",
	}, search.Configured.Value.List)

	bare := liveWeb()
	bare.Config.Policy.SearchProvider = ""
	bare.Config.Policy.SearchEndpoint = false
	bare.Config.Policy.CSEEndpoint = false
	bare.Config.Policy.CSEID = false
	assert.Equal(t, []string{
		"provider=default",
		"max_results=5",
		"search_url=unset",
		"google_cse_url=unset",
		"google_cse_id=unset",
	}, fieldByKey(t, webFields(t, bare), diag.KeyWebSearch).Configured.Value.List)
}

// The credential is three presences and nothing else: that a variable is
// named, that it holds something, and that the next request would carry it.
// No value, no hash, no suffix, no length, and not the variable's name.
func TestTheWebCredentialIsAPresenceAndNeverAValue(t *testing.T) {
	held := fieldByKey(t, webFields(t, liveWeb()), diag.KeyWebCredential)
	assert.True(t, held.Configured.Value.Bool, "the file names a variable to read the key from")
	assert.True(t, held.Loaded.Value.Bool, "and on this machine it holds something")
	assert.True(t, held.Effective.Value.Bool, "so the next search would carry a credential")
	assert.Equal(t, diag.SourceEnv, held.Loaded.Source.Kind)
	assert.Empty(t, held.Configured.Value.Str, "a credential has no string layer to be written into")
	assert.Empty(t, held.Loaded.Value.List)

	named := liveWeb()
	named.Runtime.Credential = false
	missing := fieldByKey(t, webFields(t, named), diag.KeyWebCredential)
	assert.True(t, missing.Configured.Value.Bool)
	assert.False(t, missing.Loaded.Value.Bool,
		"a variable named and empty is the failure worth telling apart from one never named")
}

// Where fetched documents land is a kind and never a path, and the one kind
// that matters most is the absent one: it leaves the web tool out entirely,
// however enabled the policy is.
func TestTheWebCacheSaysWhereWithoutSayingWhere(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind diag.WebCacheKind
	}{
		{name: "the layout's own directory", kind: diag.WebCacheDefault},
		{name: "one the configuration pointed elsewhere", kind: diag.WebCacheCustom},
		{name: "none at all", kind: diag.WebCacheUnset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := liveWeb()
			state.Config.Cache = tc.kind
			cache := fieldByKey(t, webFields(t, state), diag.KeyWebCache)
			assert.Equal(t, string(tc.kind), cache.Configured.Value.Str)
			assert.NotContains(t, cache.Configured.Source.Ref, "/",
				"the source explains the kinds and names no directory")
		})
	}
}

// The quarantine is cozyphi's own defense rather than the library's, so the
// configured layer speaks the mode's vocabulary and the two below it are the
// boolean the engine turned it into.
func TestTheQuarantineIsAModeAboveAndABooleanBelow(t *testing.T) {
	on := fieldByKey(t, webFields(t, liveWeb()), diag.KeyWebQuarantine)
	assert.Equal(t, "reader", on.Configured.Value.Str)
	assert.True(t, on.Loaded.Value.Bool)
	assert.True(t, on.Effective.Value.Bool)

	state := liveWeb()
	state.Config.Quarantine = "off"
	state.Runtime.Quarantine = false
	off := fieldByKey(t, webFields(t, state), diag.KeyWebQuarantine)
	assert.Equal(t, "off", off.Configured.Value.Str)
	assert.False(t, off.Effective.Value.Bool,
		"a fetched fragment reaches this session directly, still inside the untrusted frame")
}

// A web layer nobody wired is unavailable on every layer. An unwired owner
// rendering as "off" would be the worst answer this view can give: it reads
// as a session that cannot reach the network when it may well be able to.
func TestAnUnobservedWebLayerIsUnavailableRatherThanOff(t *testing.T) {
	fields := webFields(t, diag.WebState{})
	for _, key := range webKeys {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Configured.State, key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Empty(t, field.Revision, "an unobserved pair of owners has no state to fingerprint: %s", key)
	}

	// One owner short is not the same answer: a headless run that has read
	// the configuration but has not built an engine yet still knows what the
	// file asked for.
	half := webFields(t, diag.WebState{Config: liveWeb().Config})
	state := fieldByKey(t, half, diag.KeyWebState)
	assert.Equal(t, diag.StatePresent, state.Configured.State)
	assert.Equal(t, diag.StateUnavailable, state.Loaded.State)
	assert.Equal(t, diag.StateUnavailable, state.Effective.State,
		"with no engine there is nothing that acts, and what would act is not guessed at")
}

// Web has two owners, so neither of them can fingerprint the pair. The
// revision is composed where both are in scope, and it changes when either
// half does.
func TestTheWebRevisionFingerprintsBothOwnersAtOnce(t *testing.T) {
	fields := webFields(t, liveWeb())
	for _, key := range webKeys {
		assert.Equal(t, "ready.t1.a2.d1", fieldByKey(t, fields, key).Revision,
			"every field of one web observation describes that one observation: %s", key)
	}

	rebound := liveWeb()
	rebound.Runtime.ToolPresent = false
	rebound.Runtime.Policy.CacheSet = false
	assert.Equal(t, "no_cache_dir.t0.a2.d1",
		fieldByKey(t, webFields(t, rebound), diag.KeyWebState).Revision,
		"a snapshot taken across a rebind is visibly a snapshot of another state")
}

// The seam is an allowlist by construction. A host, a scheme, a content type,
// a cache path, a user agent, a search address, a custom-search id or an API
// key cannot be leaked by a view that has no member to put one in — so the
// shape of these structs is the guarantee, and this is the test that notices
// when someone widens one.
func TestNoWebFactHasSomewhereForASecretToLand(t *testing.T) {
	for _, tc := range []struct {
		facts   reflect.Type
		allowed map[string]string
	}{
		{
			facts: reflect.TypeFor[diag.WebPolicyFacts](),
			allowed: map[string]string{
				"Enabled":                "bool",
				"CacheSet":               "bool",
				"Schemes":                "int",
				"SchemesConfigured":      "bool",
				"AllowedHosts":           "int",
				"DeniedHosts":            "int",
				"ContentTypes":           "int",
				"ContentTypesConfigured": "bool",
				"MaxSourceBytes":         "int64",
				"TimeoutSeconds":         "int",
				"MaxRedirects":           "int",
				"CustomUserAgent":        "bool",
				// The one string, and it is out of the library's own
				// vocabulary of providers rather than anything a user wrote.
				"SearchProvider":   "string",
				"MaxSearchResults": "int",
				"SearchEndpoint":   "bool",
				"CSEEndpoint":      "bool",
				"CSEID":            "bool",
				"CredentialEnv":    "bool",
			},
		},
		{
			facts: reflect.TypeFor[diag.WebConfigFacts](),
			allowed: map[string]string{
				"Known":  "bool",
				"Policy": "diag.WebPolicyFacts",
				// A mode out of a closed set the loader refuses to widen.
				"Quarantine": "string",
				"Cache":      "diag.WebCacheKind",
			},
		},
		{
			facts: reflect.TypeFor[diag.WebRuntimeFacts](),
			allowed: map[string]string{
				"Known":       "bool",
				"Policy":      "diag.WebPolicyFacts",
				"ToolPresent": "bool",
				"Quarantine":  "bool",
				"Credential":  "bool",
			},
		},
	} {
		t.Run(tc.facts.Name(), func(t *testing.T) {
			assert.Len(t, tc.allowed, tc.facts.NumField(),
				"a new member here is a new way for an egress rule or a key to reach a transcript")
			for field := range tc.facts.Fields() {
				want, ok := tc.allowed[field.Name]
				require.True(t, ok,
					"undeclared member %q: state why it cannot carry a host or a secret before adding it",
					field.Name)
				assert.Equal(t, want, field.Type.String(), field.Name)
			}
		})
	}
}

// The prose the category carries is read by whoever is deciding whether to
// trust the answer, so it has to say what is deliberately missing from it —
// otherwise a reader takes the counts for the lists.
func TestTheWebFieldsExplainWhatTheyRefuseToCarry(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{Web: liveWeb}))

	var reason string
	for _, entry := range registry.Catalog().Categories {
		if entry.Category == diag.CategoryIntegrations {
			reason = entry.Reason
		}
	}
	for _, refused := range []string{"host", "scheme", "cache path", "user agent", "search address", "credential"} {
		assert.Contains(t, reason, refused, "the category says which web literal it does not carry")
	}
	assert.True(t, strings.HasSuffix(reason, "runs a hook"),
		"and the reason reaches the reader whole: it is bounded like any other string on the way out, "+
			"and a reason cut in half loses exactly the half that lists what is withheld")

	for _, key := range webKeys {
		explained, err := registry.Explain(t.Context(), diag.CategoryIntegrations, key)
		require.NoError(t, err, key)
		field := explained.Field
		for _, observation := range []diag.Observation{field.Configured, field.Loaded, field.Effective} {
			require.NotEmpty(t, observation.Source.Ref, "every web layer says what it is reporting: %s", key)
			assert.NotContains(t, observation.Source.Ref, "://",
				"and none of them reaches for an example address: %s", key)
		}
	}
}
