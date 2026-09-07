package agent

import (
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

// WebObservation projects this engine's web layer into the diagnostics DTO:
// the policy it was built with, whether a web tool came out of it at all,
// whether the quarantine reader stands in front of fetched text, and whether
// the credential the policy names resolves on this machine.
//
// Everything here is read from what the engine already holds. Nothing fetches
// a page, runs a search, resolves a host, opens the cache or builds a tool —
// asking cannot make this session reach the network, and cannot change what
// the next call would be allowed to reach.
//
// The credential is a presence and nothing else: the environment variable is
// read the same way websearch reads it, and only whether it holds anything
// leaves this function. No value, hash, suffix or length does, and neither
// does the variable's name.
func (engine *Engine) WebObservation() diag.WebRuntimeFacts {
	if engine == nil {
		return diag.WebRuntimeFacts{}
	}
	return diag.WebRuntimeFacts{
		Known:       true,
		Policy:      project.ObserveWebPolicy(engine.web.Policy),
		ToolPresent: engine.web.enabled(),
		Quarantine:  engine.web.Quarantine,
		Credential:  googleKeyFromEnv(engine.web.Policy) != "",
	}
}
