package profiling

import (
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Observe reports the profiling endpoint for the harness view: whether one
// is asked for, how far the address reaches, and whether the listener is up.
//
// It reads the environment and this process's own record of what it started.
// It makes no connection, fetches no profile and reads no handler — asking
// whether profiles are being served must not be a way to collect one. The
// address never travels: how exposed it is is the question worth answering,
// and the host and port are not needed to answer it.
//
// The environment is read now and the endpoint was started once, which is
// why they can disagree: an address exported into a running process is asked
// for and is not being served, and that gap is the answer somebody wondering
// why the port is closed is looking for.
func Observe() diag.ProfilingFacts {
	facts := diag.ProfilingFacts{
		Known:     true,
		Requested: strings.TrimSpace(os.Getenv(EnvAddr)) != "",
		Lifecycle: diag.ProfilingOff,
	}
	if e := current(); e != nil {
		e.mu.Lock()
		facts.Exposure, facts.Lifecycle = e.exposure, e.lifecycle
		e.mu.Unlock()
	}
	facts.Revision = profilingRevision(facts)
	return facts
}

// profilingRevision fingerprints what this observation describes: what the
// environment asks for, how far it reaches and what became of the listener.
func profilingRevision(facts diag.ProfilingFacts) string {
	requested := "0"
	if facts.Requested {
		requested = "1"
	}
	exposure := string(facts.Exposure)
	if exposure == "" {
		exposure = "none"
	}
	return "r" + requested + "." + exposure + "." + string(facts.Lifecycle)
}
