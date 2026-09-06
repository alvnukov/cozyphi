package diag

import (
	"context"
)

// integrationKeys is the declared key set, in the order Collect returns them.
// One service at a time: every MCP key, then every LSP key, then every hook
// key, so a reader scanning the catalog meets one boundary before the next.
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
}

// integrationsReason states what this category answers and what it
// deliberately leaves out.
const integrationsReason = "the services this session speaks to across a boundary it does not own: " +
	"which MCP servers are configured, which source defined each one, which of them the model can " +
	"reach, and what the last exchange with each observed; which language servers this " +
	"workspace can run, which of them are on this machine, which are running and for which roots; " +
	"and which hooks were found, which directory defined each one, which the manager in force " +
	"actually holds and what event each stands in front of. " +
	"Names and states only — no command, argument, environment entry, header, URL, setting, " +
	"hook script, run path or error text, and no tool any server offers. Nothing here starts a " +
	"server, opens a connection, probes an endpoint, downloads anything, synchronizes a " +
	"workspace, runs a hook, re-reads a hook directory or asks a server what it carries: a " +
	"server nobody has called yet is reported as one, and so is a hook nothing has fired"

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
