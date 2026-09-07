package diag

import (
	"context"
)

// integrationKeys is the declared key set, in the order Collect returns them.
// One service at a time: every MCP key, then every LSP key, then every hook
// key, then every web key, so a reader scanning the catalog meets one
// boundary before the next.
var integrationKeys = []string{
	KeyMCPState,
	KeyMCPWorkspace,
	KeyMCPServers,
	KeyMCPImported,
	KeyMCPGlobal,
	KeyMCPProject,
	KeyMCPDisabled,
	KeyMCPFailed,
	KeyLSPState,
	KeyLSPWorkspace,
	KeyLSPLanguages,
	KeyLSPServers,
	KeyLSPOperations,
	KeyLSPRoots,
	KeyLSPStart,
	KeyLSPDiagnostics,
	KeyHooksState,
	KeyHooksRegistered,
	KeyHooksEvents,
	KeyHooksTools,
	KeyHooksUser,
	KeyHooksProject,
	KeyHooksBlocking,
	KeyHooksAsync,
	KeyHooksTimeout,
	KeyHooksLoad,
	KeyWebState,
	KeyWebQuarantine,
	KeyWebCache,
	KeyWebSchemes,
	KeyWebHosts,
	KeyWebFetch,
	KeyWebSearch,
	KeyWebCredential,
}

// integrationsReason states what this category answers and what it
// deliberately leaves out.
//
// It is written to fit the response bound on one string whole, because a
// reason cut in half loses its second half — and the second half is the part
// that says what the category refuses to carry. The per-service detail that
// used to sit here lives on each field's own source instead, where a reader
// who narrowed to one key finds it and where it is bounded on its own.
const integrationsReason = "the services this session speaks to across a boundary it does not own: " +
	"which MCP servers, language servers and hooks are configured, which source defined each, " +
	"which are reachable, what the last exchange observed, and what the web tool may reach. " +
	"Names, counts and states only: no command, argument, environment entry, header, URL, host, " +
	"scheme, cache path, user agent, search address, error text, tool list or credential value. " +
	"Nothing here starts a server, opens a connection, fetches a page or runs a hook"

// IntegrationDeps binds the integration collector to the owners of the
// services this session talks to. Each is optional: a process without one
// reports that layer unavailable rather than making the category fail.
type IntegrationDeps struct {
	// MCP observes the server pool through the owner's own projection. It is
	// read, never exercised: no accessor here may connect to a server, list
	// its tools, call one, or close a client.
	MCP func() MCPState
	// LSP observes the language-server manager through the owner's own
	// projection, on the same terms: no accessor here may start a server,
	// download one, synchronize a workspace, run a query or ask for
	// diagnostics.
	LSP func() LSPState
	// Hooks observes the hook manager and the load that built it, on the
	// same terms again: no accessor here may run a hook, re-read a hook
	// directory, re-parse a manifest, rebuild a manager or change hook
	// policy. Observing hooks is also not the tool loop's own pre/post
	// hooks, which keep running exactly as the executor arranges them.
	Hooks func() HooksState
	// Web observes the web tool through its two owners' projections: the
	// loader for what was configured, the engine for what it was built
	// with. It is the one boundary here whose far side is the open
	// internet, and it is read on the strictest terms of all: no accessor
	// may fetch a page, run a search, resolve a host, open the cache or
	// read a credential's value.
	Web func() WebState
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
// touches no pool, opens no manager and reads no configuration.
func (*integrationCollector) Status() Status {
	keys := make([]string, len(integrationKeys))
	copy(keys, integrationKeys)
	return Status{Availability: AvailabilityAvailable, Reason: integrationsReason, Keys: keys}
}

// Collect reads each owner once and derives every field from that one read,
// so the fields of one answer describe one moment rather than several.
func (c *integrationCollector) Collect(_ context.Context) ([]Field, error) {
	mcp := callMCPState(c.deps.MCP)
	lsp := callLSPState(c.deps.LSP)
	hooks := callHooksState(c.deps.Hooks)
	web := callWebState(c.deps.Web)
	return []Field{
		mcp.lifecycle(),
		mcp.workspace(),
		mcp.servers(),
		mcp.definedIn(KeyMCPImported, MCPOriginImported),
		mcp.definedIn(KeyMCPGlobal, MCPOriginGlobal),
		mcp.definedIn(KeyMCPProject, MCPOriginProject),
		mcp.disabled(),
		mcp.failed(),
		lsp.lifecycle(),
		lsp.workspace(),
		lsp.languages(),
		lsp.servers(),
		lsp.operations(),
		lsp.roots(),
		lsp.start(),
		lsp.diagnostics(),
		hooks.lifecycle(),
		hooks.registered(),
		hooks.events(),
		hooks.tools(),
		hooks.definedIn(KeyHooksUser, HookOriginUser),
		hooks.definedIn(KeyHooksProject, HookOriginProject),
		hooks.blocking(),
		hooks.async(),
		hooks.timeout(),
		hooks.load(),
		web.state(),
		web.quarantine(),
		web.cache(),
		web.schemes(),
		web.hosts(),
		web.fetch(),
		web.search(),
		web.credential(),
	}, nil
}

// notApplicable is NotApplicable with the owner's own account of why the
// layer does not exist. The builder in field.go carries a kind and nothing
// else, and a layer this view deliberately leaves empty is worth a reason.
func notApplicable(source Source) Observation {
	return Observation{State: StateNotApplicable, Value: NoValue(), Source: source}
}

// unknown is Unavailable with the owner's own account of why the answer is
// not knowable here. The builder in field.go carries no provenance, and an
// answer this view deliberately refuses to guess at is worth a reason —
// otherwise "unavailable" reads as a wiring gap rather than as the point.
func unknown(source Source) Observation {
	return Observation{State: StateUnavailable, Value: NoValue(), Source: source}
}

// everyLayer sets all three layers to one observation — what a field says
// when the state of the whole subsystem, and not this field, is the answer.
func (f Field) everyLayer(observation Observation) Field {
	f.Configured, f.Loaded, f.Effective = observation, observation, observation
	return f
}
