package diag

import "strconv"

// Web field keys. Web is one more service this session speaks to across a
// boundary it does not own — the only one whose far side is the open
// internet — so it takes its own key namespace beside mcp, lsp and hooks,
// and a key stays a stable address for Explain as the category grows.
//
//nolint:gosec // G101: field addresses, not credentials; no value is one.
const (
	KeyWebState      = "web.state"
	KeyWebQuarantine = "web.quarantine"
	KeyWebCache      = "web.cache"
	KeyWebSchemes    = "web.egress.schemes"
	KeyWebHosts      = "web.egress.hosts"
	KeyWebFetch      = "web.fetch"
	KeyWebSearch     = "web.search"
	KeyWebCredential = "web.search.credential"
)

// WebLifecycle is what an engine's web configuration amounts to. The values
// separate the two different things "this session has no web tool" can mean,
// because they are fixed in two different places.
type WebLifecycle string

// WebLifecycle values.
const (
	// WebOff means network access is switched off, which is also what a
	// config file with no web: section leaves it as.
	WebOff WebLifecycle = "off"
	// WebNoCacheDir means network access is on and no cache directory is
	// set, so no tool is built at all: a fetched document would have
	// nowhere to land.
	WebNoCacheDir WebLifecycle = "no_cache_dir"
	// WebReady means a web tool is carried and a call could reach out.
	WebReady WebLifecycle = "ready"
)

// WebCacheKind names where fetched documents are cached without naming the
// directory: the layout's own, one the configuration pointed elsewhere, or
// none at all.
type WebCacheKind string

// WebCacheKind values.
const (
	// WebCacheUnset means no directory is set and no web tool can be built.
	WebCacheUnset WebCacheKind = "unset"
	// WebCacheDefault means the one this installation's layout fixes.
	WebCacheDefault WebCacheKind = "default"
	// WebCacheCustom means web.cache_dir pointed somewhere else. Where is
	// not reported: it is a path somebody wrote.
	WebCacheCustom WebCacheKind = "custom"
)

// WebHostPolicy is the shape of the host rules, which is the publishable
// part of them: whether egress is narrowed to a list somebody wrote, or open
// to everything the deny list does not stop.
type WebHostPolicy string

// WebHostPolicy values.
const (
	// WebHostsOpen means no allow-list exists, so any host the deny list
	// does not name may be reached.
	WebHostsOpen WebHostPolicy = "open"
	// WebHostsAllowList means an allow-list is in force and nothing outside
	// it is reachable, however permissive the rest of the policy is.
	WebHostsAllowList WebHostPolicy = "allow_list"
)

// WebPolicyFacts is one web policy reduced to what the harness may say about
// it. It is an allowlist by construction: there is no member for a host, a
// scheme, a content type, a cache path, a user-agent string, a search
// endpoint, a custom-search id or an API key to land in — each of them is
// reduced to a count, a kind or a presence before it leaves its owner, so no
// literal any of those lists carries can reach this view.
//
// Both owners of a web policy fill one: the loader for what was configured,
// the engine for what it was built with. They are the same shape because the
// question is the same one, asked of two layers.
type WebPolicyFacts struct {
	// Enabled is whether the policy switches network access on.
	Enabled bool
	// CacheSet is whether a cache directory is set at all. Without one no
	// web tool is built, however enabled the policy is.
	CacheSet bool
	// Schemes is how many URL schemes are admitted, and SchemesConfigured
	// whether a list was written at all — an unwritten one leaves the
	// library's own set in force, which a count of zero would misreport.
	Schemes           int
	SchemesConfigured bool
	// AllowedHosts and DeniedHosts are how many entries each host list
	// holds. Counts only: both are written by the user and can name any
	// machine on any network they can see.
	AllowedHosts int
	DeniedHosts  int
	// ContentTypes is how many response content types are accepted, and
	// ContentTypesConfigured whether a list was written, on the same terms
	// as the schemes above.
	ContentTypes           int
	ContentTypesConfigured bool
	// MaxSourceBytes, TimeoutSeconds and MaxRedirects are what bounds one
	// fetch. A value that is not positive means the library substitutes its
	// own default at the point of use.
	MaxSourceBytes int64
	TimeoutSeconds int
	MaxRedirects   int
	// CustomUserAgent is whether a user agent was supplied. Presence only:
	// the string is written by the user and is sent to every host reached.
	CustomUserAgent bool
	// SearchProvider is which provider search goes through. It is a name
	// out of the library's own vocabulary, not an address.
	SearchProvider string
	// MaxSearchResults is how many results one search returns.
	MaxSearchResults int
	// SearchEndpoint and CSEEndpoint are whether a search address and a
	// custom-search address are set. Presence only: a search URL can carry
	// a token in its path or its query.
	SearchEndpoint bool
	CSEEndpoint    bool
	// CSEID is whether a custom search engine id is set. Presence only: it
	// identifies the user's own search engine and travels in a request.
	CSEID bool
	// CredentialEnv is whether the policy names an environment variable to
	// read the search key from. Presence only, and of the naming rather
	// than of the key: a literal key is refused by the loader and never
	// reaches a policy at all.
	CredentialEnv bool
}

// WebConfigFacts is the web: section as its owner resolved it — the policy
// the loader produced, plus the two things that are cozyphi's own rather
// than the library's: the quarantine mode, and where the cache directory
// came from.
type WebConfigFacts struct {
	// Known is false when nobody published the section. Every layer fed
	// from it then reports unavailable rather than a zero policy that would
	// read as one somebody wrote.
	Known bool
	// Policy is what the loader resolved.
	Policy WebPolicyFacts
	// Quarantine is the configured mode: reader or off.
	Quarantine string
	// Cache is where fetched documents are cached, as a kind.
	Cache WebCacheKind
}

// WebRuntimeFacts is the engine's own account of the web tool it carries: the
// policy it was built with, whether a tool was built out of it at all, and
// what a call made now would go through.
type WebRuntimeFacts struct {
	// Known is false when no engine is attached to this surface yet.
	Known bool
	// Policy is the policy this engine was built with, which is not
	// necessarily the one the configuration resolved.
	Policy WebPolicyFacts
	// ToolPresent is whether this engine carries a web tool. A policy that
	// is switched off, or one with no cache directory, leaves it out
	// entirely — no call can reach the network without one.
	ToolPresent bool
	// Quarantine is whether the tool-less reader stands between a fetched
	// page and this session's context.
	Quarantine bool
	// Credential is whether the environment variable the policy names holds
	// a value. Presence only: no value, no hash, no suffix, no length.
	Credential bool
}

// WebState is one observation of the web layer, taken from both its owners:
// the loader for what was configured, the engine for what acts.
type WebState struct {
	Config  WebConfigFacts
	Runtime WebRuntimeFacts
}

// Sources for the web layers. Each one says what it is reporting and, where
// it refuses to report something, why — a count instead of a list, a
// presence instead of an address, a kind instead of a path.
var (
	sourceWebEnabled = Source{
		Kind: SourceConfigFile,
		Ref: "whether web.enabled switches network access on; a configuration with no web: section " +
			"leaves it off, because reaching the open internet is not a capability an existing config " +
			"should gain by upgrading",
	}
	sourceWebEngineEnabled = Source{
		Kind: SourceSession,
		Ref:  "the policy switch this session's engine was built with",
	}
	sourceWebLive = Source{
		Kind: SourceSession,
		Ref: "what the engine came up as: switched off, switched on with nowhere to cache, or carrying " +
			"a web tool a call could reach out with",
	}
	sourceWebNoTool = Source{
		Kind: SourceSession,
		Ref:  "this session carries no web tool, so nothing here bounds anything",
	}
	sourceWebQuarantineConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "web.quarantine: reader hands a fetched fragment to a tool-less child model call and lets " +
			"only its answer into this session; off hands the fragment straight to the session, still " +
			"inside the untrusted frame",
	}
	sourceWebQuarantineLoaded = Source{
		Kind: SourceSession,
		Ref:  "whether the engine was built with the reader standing in front of fetched text",
	}
	sourceWebQuarantineLive = Source{
		Kind: SourceSession,
		Ref:  "whether a page read now would reach this session through the reader",
	}
	sourceWebCacheConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "where fetched documents are cached, as a kind and never as a path: this installation's own " +
			"directory, one web.cache_dir pointed elsewhere, or none at all",
	}
	sourceWebCacheLoaded = Source{
		Kind: SourceSession,
		Ref: "whether the policy the engine was built with has a cache directory; without one no web tool " +
			"is built, however enabled the policy is",
	}
	sourceWebSchemesConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "how many URL schemes web.allowed_schemes admits; the list is written by the user and is " +
			"counted rather than quoted",
	}
	sourceWebSchemesDefault = Source{
		Kind: SourceConfigFile,
		Ref: "web.allowed_schemes names none, so the library's own set is in force and this view does " +
			"not carry it",
	}
	sourceWebSchemesLoaded = Source{
		Kind: SourceSession,
		Ref:  "how many schemes the policy the engine was built with admits",
	}
	sourceWebSchemesLoadedDefault = Source{
		Kind: SourceSession,
		Ref:  "the policy the engine was built with names no scheme, so the library's own set is in force",
	}
	sourceWebHostsConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "the shape of the rules web.allowed_hosts and web.denied_hosts set: how many entries each " +
			"list holds and whether an allow-list narrows egress at all. Hosts are counted, never named — " +
			"a host list is written by the user and can name any machine on any network they can see",
	}
	sourceWebHostsLoaded = Source{
		Kind: SourceSession,
		Ref:  "the shape of the host rules the engine was built with, on the same terms",
	}
	sourceWebFetchConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "what bounds one fetch: the byte ceiling, the timeout, the redirect limit, how many content " +
			"types are accepted and whether a user agent was supplied. A bound that is not positive is the " +
			"library's own default; the user agent travels as a kind, because the string itself is written " +
			"by the user and is sent to every host reached",
	}
	sourceWebFetchLoaded = Source{
		Kind: SourceSession,
		Ref:  "the same bounds, as the engine was built with them",
	}
	sourceWebSearchConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "what search is configured to be: which provider, how many results, and whether a search " +
			"address, a custom-search address and a custom-search id are set. The three travel as presence " +
			"and never as values — a search URL can carry a token in its path or its query, and the id " +
			"names the user's own search engine",
	}
	sourceWebSearchLoaded = Source{
		Kind: SourceSession,
		Ref:  "the same, as the engine was built with it",
	}
	sourceWebCredentialConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "whether web.google_api_key_env names an environment variable to read the search key from. " +
			"Presence only: neither the variable nor its value is read into this view. A literal " +
			"web.google_api_key is refused by the loader and never reaches a policy at all",
	}
	sourceWebCredentialLoaded = Source{
		Kind: SourceEnv,
		Ref: "whether the environment variable the policy names holds a value. Presence only: the value " +
			"is never read into this view, and neither is a hash, a suffix or a length of it",
	}
	sourceWebCredentialLive = Source{
		Kind: SourceSession,
		Ref:  "whether the next search request would carry a credential. Presence only",
	}
)

// lifecycle separates the two ways an engine ends up with no web tool: the
// policy is switched off, or it is on and has nowhere to cache what it would
// fetch. They are fixed in different places, and one of them looks like a bug
// from the outside while the other is a setting.
func (f WebRuntimeFacts) lifecycle() WebLifecycle {
	switch {
	case !f.Policy.Enabled:
		return WebOff
	case !f.Policy.CacheSet:
		return WebNoCacheDir
	default:
		return WebReady
	}
}

// hostPolicy is the shape of the host rules: an allow-list narrows egress to
// what it names, and without one everything the deny list misses is
// reachable.
func (f WebPolicyFacts) hostPolicy() WebHostPolicy {
	if f.AllowedHosts > 0 {
		return WebHostsAllowList
	}
	return WebHostsOpen
}

// hostLabels renders the host rules as a shape rather than as a ruleset.
func (f WebPolicyFacts) hostLabels() []string {
	return []string{
		"allowed=" + strconv.Itoa(f.AllowedHosts),
		"denied=" + strconv.Itoa(f.DeniedHosts),
		"policy=" + string(f.hostPolicy()),
	}
}

// fetchLabels renders what bounds one fetch, in one order so two answers
// about one policy read the same way.
func (f WebPolicyFacts) fetchLabels() []string {
	return []string{
		"max_source_bytes=" + webBound(f.MaxSourceBytes),
		"timeout_seconds=" + webBound(int64(f.TimeoutSeconds)),
		"max_redirects=" + webBound(int64(f.MaxRedirects)),
		"content_types=" + webCount(f.ContentTypes, f.ContentTypesConfigured),
		"user_agent=" + webUserAgent(f.CustomUserAgent),
	}
}

// searchLabels renders what search is, without the two addresses and the id
// that would say where it goes and on whose behalf.
func (f WebPolicyFacts) searchLabels() []string {
	return []string{
		"provider=" + webProvider(f.SearchProvider),
		"max_results=" + webBound(int64(f.MaxSearchResults)),
		"search_url=" + webPresence(f.SearchEndpoint),
		"google_cse_url=" + webPresence(f.CSEEndpoint),
		"google_cse_id=" + webPresence(f.CSEID),
	}
}

// webBound renders one numeric bound. A value that is not positive is not a
// bound of zero: it is the one the library substitutes at the point of use,
// and reporting it as zero would read as a fetch that may download nothing.
func webBound(n int64) string {
	if n <= 0 {
		return "default"
	}
	return strconv.FormatInt(n, 10)
}

// webCount renders how many entries a list holds, or says the list was never
// written and the library's own is in force.
func webCount(n int, configured bool) string {
	if !configured {
		return "default"
	}
	return strconv.Itoa(n)
}

// webPresence renders whether something is set, and nothing about what it is.
func webPresence(set bool) string {
	if set {
		return "set"
	}
	return "unset"
}

// webUserAgent renders which of the two user agents is sent, never the
// string: a custom one is written by the user and reaches every host.
func webUserAgent(custom bool) string {
	if custom {
		return "custom"
	}
	return "default"
}

// webProvider renders which provider search goes through. An unnamed one is
// the library's own choice rather than no search at all.
func webProvider(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

// state is web access as a whole: what the configuration asked for, what the
// engine was built with, and what it came up as.
func (s WebState) state() Field {
	field := s.field(KeyWebState)
	if s.Config.Known {
		field.Configured = Present(BoolValue(s.Config.Policy.Enabled), sourceWebEnabled)
	}
	if s.Runtime.Known {
		field.Loaded = Present(BoolValue(s.Runtime.Policy.Enabled), sourceWebEngineEnabled)
		field.Effective = Present(StringValue(string(s.Runtime.lifecycle())), sourceWebLive)
	}
	return field
}

// quarantine is what happens to a fetched page on its way into the context.
// It is cozyphi's own defense layer and has no meaning inside the library, so
// the configured layer speaks the mode's vocabulary and the two below it are
// the boolean the engine turned it into.
func (s WebState) quarantine() Field {
	field := s.field(KeyWebQuarantine)
	if s.Config.Known {
		field.Configured = Present(StringValue(s.Config.Quarantine), sourceWebQuarantineConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(BoolValue(s.Runtime.Quarantine), sourceWebQuarantineLoaded)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = Present(BoolValue(s.Runtime.Quarantine), sourceWebQuarantineLive)
	return field
}

// cache is where a fetched document lands. It is the one setting that stops a
// web tool from existing without switching anything off, which is why it has
// a field of its own rather than being read out of the lifecycle above.
func (s WebState) cache() Field {
	field := s.field(KeyWebCache)
	if s.Config.Known {
		field.Configured = Present(StringValue(string(s.Config.Cache)), sourceWebCacheConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(BoolValue(s.Runtime.Policy.CacheSet), sourceWebCacheLoaded)
		field.Effective = field.Loaded
	}
	return field
}

// schemes is how many URL schemes egress admits. A policy that names none
// leaves the library's own set in force, and that is reported as the absence
// it is rather than as a zero that would read as a boundary admitting
// nothing.
func (s WebState) schemes() Field {
	field := s.field(KeyWebSchemes)
	if s.Config.Known {
		field.Configured = webSchemeObservation(s.Config.Policy,
			sourceWebSchemesConfigured, sourceWebSchemesDefault)
	}
	if s.Runtime.Known {
		field.Loaded = webSchemeObservation(s.Runtime.Policy,
			sourceWebSchemesLoaded, sourceWebSchemesLoadedDefault)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = field.Loaded
	return field
}

// webSchemeObservation is the scheme count of one policy, or the statement
// that the policy names none.
func webSchemeObservation(policy WebPolicyFacts, named, unnamed Source) Observation {
	if !policy.SchemesConfigured {
		return Unset(NoValue(), unnamed)
	}
	return Present(IntValue(int64(policy.Schemes)), named)
}

// hosts is the shape of the host rules. Which hosts they name is the one
// thing a reader might want and the one thing that cannot travel: both lists
// are written by the user and name machines on networks this view knows
// nothing about.
func (s WebState) hosts() Field {
	field := s.field(KeyWebHosts)
	if s.Config.Known {
		field.Configured = Present(ListValue(s.Config.Policy.hostLabels()), sourceWebHostsConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(ListValue(s.Runtime.Policy.hostLabels()), sourceWebHostsLoaded)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = field.Loaded
	return field
}

// fetch is what bounds one download: how much of a page is read, how long it
// is waited for, how far a redirect chain may run, what content types are
// taken and which user agent is sent.
func (s WebState) fetch() Field {
	field := s.field(KeyWebFetch)
	if s.Config.Known {
		field.Configured = Present(ListValue(s.Config.Policy.fetchLabels()), sourceWebFetchConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(ListValue(s.Runtime.Policy.fetchLabels()), sourceWebFetchLoaded)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = field.Loaded
	return field
}

// search is what a search goes through and how much it returns. The two
// addresses and the custom-search id travel as presence, so a reader learns
// that search is configured without learning where it points.
func (s WebState) search() Field {
	field := s.field(KeyWebSearch)
	if s.Config.Known {
		field.Configured = Present(ListValue(s.Config.Policy.searchLabels()), sourceWebSearchConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(ListValue(s.Runtime.Policy.searchLabels()), sourceWebSearchLoaded)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = field.Loaded
	return field
}

// credential is whether search carries a key anywhere. It is three booleans:
// one the configuration names a variable for, one that variable actually
// holds, and one the next request would carry. No value of any of them ever
// reaches this view, and neither does the name of the variable.
func (s WebState) credential() Field {
	field := s.field(KeyWebCredential)
	if s.Config.Known {
		field.Configured = Present(BoolValue(s.Config.Policy.CredentialEnv), sourceWebCredentialConfigured)
	}
	if s.Runtime.Known {
		field.Loaded = Present(BoolValue(s.Runtime.Credential), sourceWebCredentialLoaded)
	}
	if absent, ok := s.noTool(); ok {
		field.Effective = absent
		return field
	}
	field.Effective = Present(BoolValue(s.Runtime.Credential), sourceWebCredentialLive)
	return field
}

// field is the shape every web field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero policy that would read as a boundary somebody set.
//
// The whole namespace applies on restart, and it is fixed here rather than
// passed in per field: the policy is read once when the tool is built, so
// there is no web setting a reload picks up and none that takes effect at the
// next call.
func (s WebState) field(key string) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      ApplyRestart,
		Scope:      ScopeWorkspace,
		Revision:   s.revision(),
	}
}

// revision fingerprints what this observation describes: what the web layer
// came up as, whether a tool is carried at all, and how wide the host rules
// it would run under are. Neither owner counts these — a policy keeps no
// counter — so it is only a way to see that two snapshots taken across a
// reload or a rebind are of two different states.
//
// It is composed here rather than by an owner because web has two of them,
// and neither can fingerprint a pair it cannot see. A web layer nobody wired
// at all reports no revision rather than one describing a zero policy.
func (s WebState) revision() string {
	if !s.Config.Known && !s.Runtime.Known {
		return ""
	}
	state, tool, policy := "unobserved", "0", s.Config.Policy
	if s.Runtime.Known {
		state, policy = string(s.Runtime.lifecycle()), s.Runtime.Policy
		if s.Runtime.ToolPresent {
			tool = "1"
		}
	}
	return state + ".t" + tool +
		".a" + strconv.Itoa(policy.AllowedHosts) + ".d" + strconv.Itoa(policy.DeniedHosts)
}

// noTool reports whether an effective layer has anything to describe, and
// what to say when it does not. The two cases are not the same: an engine
// nobody attached cannot say what would bound a fetch, while one that carries
// no web tool has answered — no fetch happens at all, so no bound applies.
func (s WebState) noTool() (Observation, bool) {
	switch {
	case !s.Runtime.Known:
		return Unavailable(), true
	case !s.Runtime.ToolPresent:
		return notApplicable(sourceWebNoTool), true
	default:
		return Observation{}, false
	}
}

// callWebState reads the optional accessor. A nil accessor is a wiring gap,
// and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callWebState(accessor func() WebState) WebState {
	if accessor == nil {
		return WebState{}
	}
	return accessor()
}
