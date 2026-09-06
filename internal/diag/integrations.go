package diag

import (
	"context"
	"slices"
)

// MCP field keys. The category holds every service cozyphi speaks to over a
// boundary it does not own, so each service takes its own key namespace and
// a key stays a stable address for Explain as the category grows.
//
//nolint:gosec // G101: field addresses, not credentials; no value is one.
const (
	KeyMCPState     = "mcp.state"
	KeyMCPWorkspace = "mcp.workspace"
	KeyMCPServers   = "mcp.servers"
	KeyMCPImported  = "mcp.source.imported"
	KeyMCPGlobal    = "mcp.source.global"
	KeyMCPProject   = "mcp.source.project"
	KeyMCPDisabled  = "mcp.disabled"
	KeyMCPFailed    = "mcp.failed"
)

// integrationKeys is the declared key set, in the order Collect returns them.
var integrationKeys = []string{
	KeyMCPState,
	KeyMCPWorkspace,
	KeyMCPServers,
	KeyMCPImported,
	KeyMCPGlobal,
	KeyMCPProject,
	KeyMCPDisabled,
	KeyMCPFailed,
}

// integrationsReason states what this category answers and what it
// deliberately leaves out.
const integrationsReason = "the services this session speaks to across a boundary it does not own: " +
	"which MCP servers are configured, which source defined each one, which of them the model can " +
	"reach, and what the last exchange with each observed. Names and states only — no command, " +
	"argument, environment entry, header, URL or error text, and no tool any server offers. Nothing " +
	"here starts a server, opens a connection, probes an endpoint or asks a server what it carries: " +
	"a server nobody has called yet is reported as one"

// MCPOrigin names one configuration source a server definition came from.
// The values are also the precedence order: an imported definition is
// replaced by the global one, and the global one by the project's.
type MCPOrigin string

// MCPOrigin values, in precedence order.
const (
	MCPOriginImported MCPOrigin = "imported"
	MCPOriginGlobal   MCPOrigin = "global"
	MCPOriginProject  MCPOrigin = "project"
)

// MCPConnection is the last thing the pool observed about one server. It is
// a record of what already happened, never a prediction: a server that has
// never been called is not "reachable", it is one nobody has called.
type MCPConnection string

// MCPConnection values.
const (
	// MCPServerNotConnected means no connection is open and none has failed.
	// The pool connects on the first call, so this is where every server
	// starts and where a closed pool leaves them.
	MCPServerNotConnected MCPConnection = "not_connected"
	// MCPServerConnected means the last exchange with the server succeeded.
	MCPServerConnected MCPConnection = "connected"
	// MCPServerFailed means the last spawn or exchange failed. What failed
	// is not reported: an error from a server is arbitrary text.
	MCPServerFailed MCPConnection = "failed"
	// MCPServerDisabled means the server is switched off and the model
	// cannot reach it, however well it is configured.
	MCPServerDisabled MCPConnection = "disabled"
)

// MCPLifecycle is where the MCP subsystem itself stands. The values separate
// the five different things "no servers" could otherwise mean.
type MCPLifecycle string

// MCPLifecycle values.
const (
	// MCPEnabled means the environment leaves MCP on. It is the configured
	// layer's answer, and says nothing about whether anything loaded.
	MCPEnabled MCPLifecycle = "enabled"
	// MCPDisabled means the environment switched MCP off, so no pool exists
	// and nothing below it applies.
	MCPDisabled MCPLifecycle = "disabled"
	// MCPLoaded means a pool was built from the configuration.
	MCPLoaded MCPLifecycle = "loaded"
	// MCPNotLoaded means nothing published a pool — no load was attempted,
	// or one was attempted and produced nothing.
	MCPNotLoaded MCPLifecycle = "not_loaded"
	// MCPLoadFailed means reading the configuration failed. The servers a
	// working file would have defined are not known, and are not guessed.
	MCPLoadFailed MCPLifecycle = "load_failed"
	// MCPNotConfigured means the pool loaded and no source named a server.
	MCPNotConfigured MCPLifecycle = "not_configured"
	// MCPReady means the pool holds servers the model can ask for.
	MCPReady MCPLifecycle = "ready"
	// MCPClosed means the pool was shut down; no server can be reached again
	// without a new one.
	MCPClosed MCPLifecycle = "closed"
)

// MCPServerFacts is what the harness may say about one configured MCP
// server. It is an allowlist by construction: a name the user chose, where
// it was defined, and four states — with no member for a command, an
// argument, an environment entry, a header, a URL, an error message or a
// tool schema to land in.
type MCPServerFacts struct {
	// Name is the key the server is configured under. It is already in the
	// model's system prompt, which is what makes it safe to name here.
	Name string
	// Origins is every configuration source that defines this name, in
	// precedence order, so the last one is the definition in force.
	Origins []MCPOrigin
	// Usable is whether the definition could produce a connection at all —
	// a command to run or a URL to reach, over a transport this client
	// speaks. What is wrong with an unusable one is not reported.
	Usable bool
	// Enabled is whether the model may reach the server right now.
	Enabled bool
	// OffInConfig is whether the configuration files started it switched
	// off, which is what tells a persisted choice from this session's.
	OffInConfig bool
	// Connection is the last state the pool observed for this server.
	Connection MCPConnection
}

// MCPState is one observation of the MCP layer: whether the subsystem is on,
// whether a pool was built, where its servers run, and one entry per
// configured server.
type MCPState struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty. Every field it feeds then reports
	// unavailable instead of an empty list that would read as "no servers".
	Known bool
	// Enabled is whether the environment leaves MCP on.
	Enabled bool
	// Loaded is whether a pool exists.
	Loaded bool
	// LoadFailed is whether reading the configuration failed.
	LoadFailed bool
	// Closed is whether the pool has been shut down.
	Closed bool
	// Workspace is the directory a server started as a local process is
	// spawned in. Empty means cozyphi's own directory is inherited.
	Workspace string
	// Servers is one entry per configured server, sorted by name.
	Servers []MCPServerFacts
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a toggle or a first call are visibly of two
	// different states.
	Revision string
}

// Sources for the layers whose origin is the view's own reading rather than
// an owner's record.
var (
	sourceMCPEnv = Source{
		Kind: SourceEnv,
		Ref:  "COZYPHI_MCP",
	}
	sourceMCPDefault = Source{
		Kind: SourceDefault,
		Ref:  "MCP is on unless COZYPHI_MCP switches it off",
	}
	sourceMCPLoad = Source{
		Kind: SourceComputed,
		Ref:  "whether a pool was built from the configuration when this workspace opened",
	}
	sourceMCPPool = Source{
		Kind: SourceSession,
		Ref:  "the MCP pool this workspace holds",
	}
	sourceMCPOff = Source{
		Kind: SourceEnv,
		Ref:  "COZYPHI_MCP switched MCP off, so there is no pool for this to describe",
	}
	sourceMCPConfigured = Source{
		Kind: SourceConfigFile,
		Ref: "every server the sources this pool was built from define, merged in precedence " +
			"order: imported, then global, then project",
	}
	sourceMCPReachable = Source{
		Kind: SourceSession,
		Ref:  "the servers the pool exposes to the model — configured and not switched off",
	}
	sourceMCPConnected = Source{
		Kind: SourceSession,
		Ref: "the servers whose last exchange succeeded; the rest of the loaded layer has been " +
			"called and failed, or not called at all — a connection is opened by a call, never here",
	}
	sourceMCPDefines = Source{
		Kind: SourceConfigFile,
		Ref:  "the servers this source names, whether or not its definition is the one in force",
	}
	sourceMCPWins = Source{
		Kind: SourceConfigFile,
		Ref:  "the servers whose definition came from this source — the highest-precedence one naming them",
	}
	sourceMCPPrecedence = Source{
		Kind: SourceConfigFile,
		Ref:  "which source defined a server is not something that acts; what acts is its definition",
	}
	sourceMCPDisabledInConfig = Source{
		Kind: SourceConfigFile,
		Ref:  `the "disabled" list of the mcp.json files, as it read when the pool was built`,
	}
	sourceMCPDisabledNow = Source{
		Kind: SourceSession,
		Ref:  "the servers switched off right now, this session's own /mcp toggles included",
	}
	sourceMCPUnusable = Source{
		Kind: SourceConfigFile,
		Ref: "the servers whose definition cannot produce a connection at all — nothing to run, " +
			"nothing to reach, or a transport this client does not speak",
	}
	sourceMCPFailed = Source{
		Kind: SourceSession,
		Ref:  "the servers whose last spawn or exchange failed; what failed is not reported here",
	}
	sourceMCPFailureLayer = Source{
		Kind: SourceSession,
		Ref:  "a failure is observed, not loaded: it belongs to the call that met it",
	}
	sourceMCPWorkspace = Source{
		Kind: SourceSession,
		Ref:  "the directory a server started as a local process is spawned in, resolved once before any client existed",
	}
	sourceMCPWorkspaceInherited = Source{
		Kind: SourceDefault,
		Ref:  "no directory was given, so a local process inherits cozyphi's own",
	}
	sourceMCPWorkspaceOwner = Source{
		Kind: SourceConfigFile,
		Ref:  "the workspace this session opened decides it; no configuration file does",
	}
)

// IntegrationDeps binds the integration collector to the owners of the
// services this session talks to. Each is optional: a process without one
// reports that layer unavailable rather than making the category fail.
type IntegrationDeps struct {
	// MCP observes the server pool through the owner's own projection. It is
	// read, never exercised: no accessor here may connect to a server, list
	// its tools, call one, or close a client.
	MCP func() MCPState
}

// integrationCollector observes what this session reaches outside itself.
type integrationCollector struct {
	deps IntegrationDeps
}

// NewIntegrationCollector builds the integration collector.
func NewIntegrationCollector(deps IntegrationDeps) Collector {
	return &integrationCollector{deps: deps}
}

func (*integrationCollector) Category() Category { return CategoryIntegrations }

// Status is answered from the declared key set alone: listing the catalog
// touches no pool and reads no configuration.
func (*integrationCollector) Status() Status {
	keys := make([]string, len(integrationKeys))
	copy(keys, integrationKeys)
	return Status{Availability: AvailabilityAvailable, Reason: integrationsReason, Keys: keys}
}

// Collect reads each owner once and derives every field from that one read,
// so the fields of one answer describe one moment rather than several.
func (c *integrationCollector) Collect(_ context.Context) ([]Field, error) {
	mcp := callMCPState(c.deps.MCP)
	return []Field{
		mcp.lifecycle(),
		mcp.workspace(),
		mcp.servers(),
		mcp.definedIn(KeyMCPImported, MCPOriginImported),
		mcp.definedIn(KeyMCPGlobal, MCPOriginGlobal),
		mcp.definedIn(KeyMCPProject, MCPOriginProject),
		mcp.disabled(),
		mcp.failed(),
	}, nil
}

// lifecycle separates the five things "no MCP servers" could mean: switched
// off, never loaded, loaded from a file that would not parse, loaded with
// nothing in it, and shut down. They call for entirely different fixes, and
// one of them is not a problem at all.
func (s MCPState) lifecycle() Field {
	field := s.field(KeyMCPState, ApplyRestart)
	if !s.Known {
		return field
	}
	setting, source := MCPEnabled, sourceMCPDefault
	if !s.Enabled {
		setting, source = MCPDisabled, sourceMCPEnv
	}
	field.Configured = Present(StringValue(string(setting)), source)
	field.Loaded = Present(StringValue(string(s.loadState())), sourceMCPLoad)
	field.Effective = Present(StringValue(string(s.liveState())), sourceMCPPool)
	return field
}

// workspace names where a server started as a local process runs. Nothing in
// a configuration file sets it — the session's own workspace does — so the
// configured layer does not exist rather than reporting a default.
func (s MCPState) workspace() Field {
	field := s.field(KeyMCPWorkspace, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	observation := Unset(StringValue(""), sourceMCPWorkspaceInherited)
	if s.Workspace != "" {
		observation = Present(StringValue(s.Workspace), sourceMCPWorkspace)
	}
	field.Configured = notApplicable(sourceMCPWorkspaceOwner)
	field.Loaded = observation
	field.Effective = observation
	return field
}

// servers is the direct answer to "what does this session actually have":
// every configured name, the subset the model may ask for, and the subset a
// call has already reached. The three shrink in that order, and the gaps are
// the answer — a name that is configured and not reachable is switched off,
// and a reachable one that is not connected has simply not been called.
func (s MCPState) servers() Field {
	field := s.field(KeyMCPServers, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(func(MCPServerFacts) bool { return true })), sourceMCPConfigured)
	field.Loaded = Present(ListValue(s.names(func(f MCPServerFacts) bool { return f.Enabled })), sourceMCPReachable)
	field.Effective = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return f.Connection == MCPServerConnected
	})), sourceMCPConnected)
	return field
}

// definedIn reports one configuration source twice over: what it names, and
// what it actually supplied. A server named by two sources appears in the
// first list of both and the second list of one, which is the whole of what
// precedence did — stated, rather than left to be inferred from a merge
// nobody watched.
func (s MCPState) definedIn(key string, origin MCPOrigin) Field {
	field := s.field(key, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return slices.Contains(f.Origins, origin)
	})), sourceMCPDefines)
	field.Loaded = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return winningOrigin(f.Origins) == origin
	})), sourceMCPWins)
	field.Effective = notApplicable(sourceMCPPrecedence)
	return field
}

// disabled tells a persisted choice from this session's: the configuration
// switched some servers off before the pool existed, and /mcp may have moved
// the set since. Where the two differ, the difference has not been written
// down yet.
func (s MCPState) disabled() Field {
	field := s.field(KeyMCPDisabled, ApplyImmediate)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	off := Present(ListValue(s.names(func(f MCPServerFacts) bool { return !f.Enabled })), sourceMCPDisabledNow)
	field.Configured = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return f.OffInConfig
	})), sourceMCPDisabledInConfig)
	field.Loaded = off
	field.Effective = off
	return field
}

// failed separates the two ways a server can be broken, because they are
// fixed in different places: a definition that could never connect is a
// configuration fault and is known without calling anything, while a server
// whose last exchange failed was reachable enough to try. Neither says what
// went wrong — an error from a server is arbitrary text, and this view
// carries none of it.
func (s MCPState) failed() Field {
	field := s.field(KeyMCPFailed, ApplyImmediate)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return !f.Usable
	})), sourceMCPUnusable)
	field.Loaded = notApplicable(sourceMCPFailureLayer)
	field.Effective = Present(ListValue(s.names(func(f MCPServerFacts) bool {
		return f.Connection == MCPServerFailed
	})), sourceMCPFailed)
	return field
}

// field is the shape every MCP field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into an empty list that would read as "nothing is configured".
func (s MCPState) field(key string, apply Apply) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      ScopeWorkspace,
		Revision:   s.Revision,
	}
}

// notApplicable is NotApplicable with the owner's own account of why the
// layer does not exist. The builder in field.go carries a kind and nothing
// else, and a layer this view deliberately leaves empty is worth a reason.
func notApplicable(source Source) Observation {
	return Observation{State: StateNotApplicable, Value: NoValue(), Source: source}
}

// everyLayer sets all three layers to one observation — what a field says
// when the state of the whole subsystem, and not this field, is the answer.
func (f Field) everyLayer(observation Observation) Field {
	f.Configured, f.Loaded, f.Effective = observation, observation, observation
	return f
}

// absent reports whether the pool can answer a per-server question at all,
// and what to say when it cannot. The two cases are not the same: a
// subsystem switched off has no servers to describe, while one that never
// loaded, or failed to, cannot say what it would have had. Off is tested
// first because switching MCP off is what stops a pool from being built —
// the missing pool is the consequence, and reporting it as the reason would
// hide an answer that is known for one that is not.
func (s MCPState) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Enabled:
		return notApplicable(sourceMCPOff), true
	case !s.Loaded:
		return Unavailable(), true
	default:
		return Observation{}, false
	}
}

// loadState is what became of the configuration: a pool, nothing, or an
// error nobody may see the text of.
func (s MCPState) loadState() MCPLifecycle {
	switch {
	case s.LoadFailed:
		return MCPLoadFailed
	case s.Loaded:
		return MCPLoaded
	default:
		return MCPNotLoaded
	}
}

// liveState is what MCP amounts to right now, answered in the order the
// answers stop mattering: off beats everything, a pool that never arrived
// beats what it would have held, and a closed one beats what it holds.
func (s MCPState) liveState() MCPLifecycle {
	switch {
	case !s.Enabled:
		return MCPDisabled
	case s.LoadFailed:
		return MCPLoadFailed
	case !s.Loaded:
		return MCPNotLoaded
	case s.Closed:
		return MCPClosed
	case len(s.Servers) == 0:
		return MCPNotConfigured
	default:
		return MCPReady
	}
}

// names collects the server names matching keep, in the order the owner
// reported them.
func (s MCPState) names(keep func(MCPServerFacts) bool) []string {
	out := make([]string, 0, len(s.Servers))
	for _, facts := range s.Servers {
		if keep(facts) {
			out = append(out, facts.Name)
		}
	}
	return out
}

// winningOrigin is the source whose definition is in force. The loader
// records sources in precedence order, so the last one recorded is the one
// that overwrote the others.
func winningOrigin(origins []MCPOrigin) MCPOrigin {
	if len(origins) == 0 {
		return ""
	}
	return origins[len(origins)-1]
}

// callMCPState reads the optional accessor. A nil accessor is a wiring gap,
// and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callMCPState(accessor func() MCPState) MCPState {
	if accessor == nil {
		return MCPState{}
	}
	return accessor()
}
