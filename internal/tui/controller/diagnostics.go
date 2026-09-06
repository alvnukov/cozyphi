package controller

import (
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/version"
)

// tuiMode is what the runtime category reports as the process shape. It is the
// counterpart of the headless run's "headless", and it is the only thing about
// the harness view that differs between the two entry points.
const tuiMode = "tui"

// newDiagnostics builds one session's read-only view of the harness, or nil
// when the process was not started with --developer-mode.
//
// One registry per user session rather than one per process: its accessors are
// bound to this controller, so switching or closing a session can never answer
// with another session's identity. The registry owns nothing and borrows
// nothing that needs closing — a session ends without taking a shared resource
// with it.
func (r *Runtime) newDiagnostics(c *Controller) *diag.Registry {
	if c == nil || !r.developerModeGranted() {
		return nil
	}
	return diag.NewRegistry(nil, diag.DefaultLimits(), diag.NewRuntimeCollector(diag.RuntimeDeps{
		Version:   version.Version,
		Mode:      tuiMode,
		Enabled:   true,
		Workspace: func() string { return c.cwd },
		SessionID: c.routingSessionID,
	}))
}

// routingSessionID reads the identity this controller publishes for routing.
// It follows every engine replacement — clear, resume, model rebind — so an
// observation names the session the model is actually running in, and reads
// empty before the first engine exists rather than naming a session that does
// not.
func (c *Controller) routingSessionID() string {
	if c == nil {
		return ""
	}
	if id := c.progressSession.Load(); id != nil {
		return *id
	}
	return ""
}
