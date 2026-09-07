package session

import (
	"fmt"
	"path/filepath"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/plantel"
)

// Observe reports the transcript's state for the harness view: whether this
// session is persisted, whether a file is held, whether anything has reached
// it, and how many entries the manager holds.
//
// It reads what the manager already holds, under the manager's own lock, and
// does nothing else. No entry is appended, no flush forced, no session
// opened or resumed, and the transcript file is neither read, stat'd nor
// listed — the counts are of memory, which is why asking cannot change what
// a later turn writes.
//
// Nothing that could carry a secret is copied out: not a message, a role, a
// title, a plan, a tool call, a model name or the transcript path itself.
// What leaves is three states and one count.
//
// dir is the session directory this workspace's layout fixes. It is compared
// by name alone, so a resumed transcript from somewhere else is reported as
// elsewhere rather than as a file in a directory it is not in.
func Observe(sm *Manager, dir string) diag.SessionStoreFacts {
	if sm == nil {
		return diag.SessionStoreFacts{}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	facts := diag.SessionStoreFacts{
		Known:      true,
		Persisting: sm.shouldFlush,
		Bound:      sm.sessionFile != "",
		Written:    sm.flushed,
		Entries:    len(sm.entries),
	}
	facts.Local = facts.Bound && local(sm.sessionFile, dir)
	facts.Revision = fmt.Sprintf("e%d.b%t.w%t", facts.Entries, facts.Bound, facts.Written)
	return facts
}

// local reports whether a transcript sits in the workspace's own session
// directory. The comparison is on the directory's name rather than its whole
// path: a session file is canonicalized when it is taken, so its prefix may
// have had symlinks resolved out of it while the layout's has not. The name
// below the base encodes the working directory and is what actually
// separates one workspace's transcripts from another's.
func local(file, dir string) bool {
	if file == "" || dir == "" {
		return false
	}
	return filepath.Base(filepath.Dir(file)) == filepath.Base(filepath.Clean(dir))
}

// ObserveTelemetry reports whether this session is counting the plan. The
// tracker is the manager's own and is not handed out; what leaves here is
// the mechanism — a tracker is in force or is not, and the schema is this
// wide — never a counter. What the counters say is the plan category's
// answer, and repeating it here would be two answers to one question.
//
// A manager without a tracker reads as telemetry switched off, which is what
// every recording method here already takes it for.
func ObserveTelemetry(sm *Manager) diag.TelemetryFacts {
	if sm == nil {
		return plantel.Observe(nil)
	}
	return plantel.Observe(sm.telemetry)
}
