package agent

import (
	"context"
	"strings"

	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/hooks"
)

// QueueSessionContext parks text a session_start hook returned. It reaches
// the model once, wrapped as a system reminder, at the next composed prompt
// or tool result. A second call before delivery replaces the first: the
// latest session start describes the session that is actually open. An
// engine without lifecycle hooks (a child, a headless run) ignores it.
func (engine *Engine) QueueSessionContext(text string) {
	if engine == nil {
		return
	}
	text = strings.TrimSpace(text)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if !engine.lifecycle {
		return
	}
	if text == "" {
		engine.sessionContext = ""
		return
	}
	// text is a hook's output, not markup: an embedded close tag must not be
	// able to forge the wrapper's end and smuggle a fake trailer past it.
	text = strings.ReplaceAll(text, reminderClose, `<\/system-reminder>`)
	engine.sessionContext = reminderOpen + "\n" + text + "\n" + reminderClose
}

// drainSessionContext takes the parked session context, if any.
func (engine *Engine) drainSessionContext() string {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	reminder := engine.sessionContext
	engine.sessionContext = ""
	return reminder
}

// drainBoundaryReminders is what the executor attaches to a tool result:
// session context first, then compaction advice.
func (engine *Engine) drainBoundaryReminders() string {
	return prependReminder(engine.drainSessionContext(), engine.drainCompactAdvice())
}

// refireSessionStart runs session_start with reason compact after a
// successful compaction or trim, so a plugin bootstrap that left the context
// is delivered again. The engine has no UI channel: toast and status are
// logged only.
func (engine *Engine) refireSessionStart(ctx context.Context) {
	// SessionID and SessionCwd take engine.mu themselves, so they are read
	// before the lock below rather than under it.
	sessionID, cwd := engine.SessionID(), engine.SessionCwd()
	engine.mu.RLock()
	mgr, lifecycle := engine.hooks, engine.lifecycle
	engine.mu.RUnlock()
	if !lifecycle || mgr == nil {
		return
	}
	out := mgr.SessionStart(ctx, hooks.SessionEvent{
		SessionID: sessionID,
		Cwd:       cwd,
		Reason:    hooks.ReasonCompact,
	})
	if out.Toast != "" || out.StatusSet {
		debuglog.Logf("hooks: compact session_start toast=%q status=%q (not shown: no UI here)", out.Toast, out.Status)
	}
	// Nothing to add for compact is not an empty session: a bootstrap from
	// startup or resume still waiting for delivery stays valid.
	if strings.TrimSpace(out.Context) == "" {
		return
	}
	engine.QueueSessionContext(out.Context)
}
