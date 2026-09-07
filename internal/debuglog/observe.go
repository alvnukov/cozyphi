package debuglog

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Observe reports the debug log for the harness view: whether the switch is
// set, whether it has been read yet and what it was read into, and the name
// of the file lines land in.
//
// It reads this package's own state under the same mutex Logf takes, and the
// environment beside it. It does not call Enabled: that would latch the
// switch, and a read of the harness that changed what the harness reports is
// not a read. Nothing opens the file, writes a line or reads one back — a
// debug line quotes whatever the process was doing, and none of it leaves
// this package.
//
// Only the base name of the file travels. Where it sits is a home directory
// or a path a script chose, and neither is needed to answer the question.
func Observe() diag.LoggingFacts {
	facts := diag.LoggingFacts{
		Known:       true,
		Requested:   switchedOn(),
		Destination: filepath.Base(Path()),
		FromEnv:     strings.TrimSpace(os.Getenv("COZYPHI_DEBUG_FILE")) != "",
		// The two subsystem logs honor a directory of their own. Presence
		// only, spelled as their owners spell it: a blank value is unset.
		MCPLogFromEnv:      strings.TrimSpace(os.Getenv("COZYPHI_MCP_LOG_DIR")) != "",
		PlanGateLogFromEnv: strings.TrimSpace(os.Getenv("COZYPHI_PLAN_GATE_LOG_DIR")) != "",
	}
	mu.Lock()
	facts.Latched, facts.Enabled = checked, enabled
	if file != nil {
		facts.OpenName = filepath.Base(file.Name())
	}
	mu.Unlock()
	facts.Revision = loggingRevision(facts)
	return facts
}

// switchedOn is what the switch says right now, spelled exactly as Enabled
// spells it. The spelling is duplicated rather than shared because Enabled
// latches and this must not.
func switchedOn() bool {
	value := os.Getenv("COZYPHI_DEBUG")
	return value == "1" || strings.EqualFold(value, "true")
}

// loggingRevision fingerprints what this observation describes: what the
// switch says, what it latched to, whether a file is open, and which
// subsystem logs are redirected. It is only a
// way to see that two answers taken across the first written line are of two
// different states.
func loggingRevision(facts diag.LoggingFacts) string {
	state := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	open := "0"
	if facts.OpenName != "" {
		open = "1"
	}
	return "r" + state(facts.Requested) + ".l" + state(facts.Latched) +
		".e" + state(facts.Enabled) + ".o" + open +
		".m" + state(facts.MCPLogFromEnv) + ".p" + state(facts.PlanGateLogFromEnv)
}

// AuditSink is where a harness request's record goes: this log, which is off
// unless somebody switched it on. The diag registry writes nothing itself —
// the sink is injected by whoever wires the view — so a read stays a read
// wherever no sink is wired.
//
// The event is an allowlist by construction: what was asked, how far it
// reached and how it ended. It has no member for a value, a source ref, a
// reason or an error message to travel in, so recording it cannot become the
// leak the rest of the view is careful not to be.
//
// Writing the line latches the switch exactly the way any other line does,
// and the record is written after the fields are collected — so a snapshot
// never reports the latch its own record caused.
func AuditSink(event diag.AuditEvent) {
	Logf("%s", event.Line())
}
