package agent

import (
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/session"
)

// SessionStoreObservation projects this engine's transcript into the
// diagnostics DTO: whether the conversation is written to a file, whether
// one is held, whether anything has reached it, and how many entries the
// manager carries.
//
// Everything here is read from what the manager already holds. No entry is
// appended, no flush forced, no session opened or resumed, and the file is
// neither read nor listed — asking cannot change what the next turn writes.
//
// dir is the session directory this workspace's layout fixes, which is what
// separates a transcript kept where this workspace's transcripts go from one
// resumed out of a path someone named.
func (engine *Engine) SessionStoreObservation(dir string) diag.SessionStoreFacts {
	if engine == nil {
		return diag.SessionStoreFacts{}
	}
	return engine.sessionRef().StoreObservation(dir)
}

// StoreObservation reports the session store's own state. The manager is
// unexported, so this is the seam the engine's observation goes through.
func (s *Session) StoreObservation(dir string) diag.SessionStoreFacts {
	if s == nil {
		return diag.SessionStoreFacts{}
	}
	return session.Observe(s.manager, dir)
}
