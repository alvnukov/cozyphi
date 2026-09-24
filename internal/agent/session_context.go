package agent

import "strings"

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
