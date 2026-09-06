package mcp

import (
	"fmt"
	"sort"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// LoadFacts is what the caller knew at the moment it loaded a pool and
// cannot recover afterwards. Whether MCP was switched off in the environment
// and whether the configuration failed to parse are both answers that exist
// once, at load, and an observation is not allowed to go looking for them
// again: re-reading the files would make asking a question change what is
// reported.
type LoadFacts struct {
	// Attempted is whether a load happened at all. A zero LoadFacts is a
	// caller that never loaded MCP, and the whole layer reports unavailable
	// rather than "nothing is configured".
	Attempted bool
	// Disabled is whether the environment switched the subsystem off.
	Disabled bool
	// Failed is whether the load returned an error.
	Failed bool
}

// ObserveLoad records the outcome of one LoadPool or LoadPoolInDir call. It
// is called where the load is, because that is the only place both answers
// are known.
//
// The error is taken and dropped on purpose. It names the file it failed on
// and, for a parse failure, can quote what it could not parse; Failed says
// that the load failed, and none of the error's text — and no fragment of
// any file it read — leaves this call.
func ObserveLoad(err error) LoadFacts {
	return LoadFacts{Attempted: true, Disabled: Disabled(), Failed: err != nil}
}

// Observe reports the pool's state for the harness view: what is configured,
// where each definition came from, what the model can reach and what the
// last exchange with each server observed.
//
// It reads what the pool already holds, under the pool's own lock, and does
// nothing else. No server is started, no connection opened, no tool list
// requested and no health endpoint probed — a server nobody has called yet
// is reported as one, which is the whole point of asking.
//
// Nothing that could carry a secret is copied out: not a command, an
// argument, an environment entry, a header, a URL, an error message or any
// tool a server offers. What leaves is a name the user chose, where it was
// defined, and a state.
func Observe(p *Pool, load LoadFacts) diag.MCPState {
	state := diag.MCPState{
		Known:      load.Attempted,
		Enabled:    !load.Disabled,
		LoadFailed: load.Failed,
		Servers:    []diag.MCPServerFacts{},
	}
	if p == nil {
		return state
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state.Loaded = true
	state.Closed = p.closed
	state.Workspace = p.cwd

	names := make([]string, 0, len(p.servers))
	for name := range p.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	state.Servers = make([]diag.MCPServerFacts, 0, len(names))
	for _, name := range names {
		state.Servers = append(state.Servers, p.observeServer(name))
	}
	state.Revision = poolRevision(state)
	return state
}

// observeServer describes one configured server. It is called with p.mu
// held, so the four states it reports are of one moment rather than of four.
func (p *Pool) observeServer(name string) diag.MCPServerFacts {
	cfg := p.servers[name]
	facts := diag.MCPServerFacts{
		Name:        name,
		Origins:     observedOrigins(p.origins[name]),
		Usable:      validateServerConfig(cfg) == nil,
		Enabled:     !p.disabled[name],
		OffInConfig: p.offInConfig[name],
		Connection:  diag.MCPServerNotConnected,
	}
	if p.disabled[name] {
		facts.Connection = diag.MCPServerDisabled
		return facts
	}
	switch p.status[name].State {
	case StateConnected:
		facts.Connection = diag.MCPServerConnected
	case StateFailed:
		facts.Connection = diag.MCPServerFailed
	case StateDisabled:
		facts.Connection = diag.MCPServerDisabled
	case StateConfigured:
		// Configured and nothing more: the pool connects on the first call,
		// and no call has been made. Reporting it as anything else would be
		// this view promising a connection it must never open.
	}
	return facts
}

// observedOrigins converts the loader's provenance into the view's, and
// detaches it: the pool keeps its slice, the caller gets its own.
func observedOrigins(origins []Origin) []diag.MCPOrigin {
	out := make([]diag.MCPOrigin, 0, len(origins))
	for _, origin := range origins {
		out = append(out, diag.MCPOrigin(origin))
	}
	return out
}

// poolRevision fingerprints what this observation describes: how many
// servers are configured, how many the model can reach and how many are
// connected. It is not a counter the pool keeps — nothing in the pool
// increments — so it is only a way to see that two snapshots taken across a
// /mcp toggle or a first call are of two different states.
func poolRevision(state diag.MCPState) string {
	reachable, connected := 0, 0
	for _, server := range state.Servers {
		if server.Enabled {
			reachable++
		}
		if server.Connection == diag.MCPServerConnected {
			connected++
		}
	}
	return fmt.Sprintf("s%d.r%d.c%d", len(state.Servers), reachable, connected)
}
