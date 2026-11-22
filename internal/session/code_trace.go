package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/redact"
	"github.com/alvnukov/cozyphi/internal/util"
)

// EntryCodeTrace is host-owned operation history outside the provider conversation.
const EntryCodeTrace = "EntryCodeTrace"

// CodeTraceEntry journals one observed native operation state.
type CodeTraceEntry struct {
	SessionBaseEntry
	Run ToolRun `json:"operation"`
}

func (CodeTraceEntry) GetType() string      { return EntryCodeTrace }
func (s CodeTraceEntry) GetID() string      { return s.ID }
func (s CodeTraceEntry) GetParent() *string { return s.ParentID }

// AppendCodeTrace records a start before dispatch and the final status after
// the operation. Bulk data stays in the guest; bounded diffs remain inspectable.
func (sm *Manager) AppendCodeTrace(run ToolRun) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	run.Detail = util.TruncateBytes(redact.Redact(run.Detail), 2048)
	run.Error = util.TruncateBytes(redact.Redact(run.Error), 2048)
	switch run.Name {
	case "edit", "write":
		run.Output = util.TruncateBytes(redact.Redact(run.Output), 16384)
	default:
		run.Output = ""
	}
	return sm.appendEntry(CodeTraceEntry{SessionBaseEntry: SessionBaseEntry{
		Type: EntryCodeTrace, ID: sm.generateID(), ParentID: sm.leafID, Timestamp: time.Now(),
	}, Run: run})
}

// ReplayEntries includes nested events for retained calls in their original
// order without adding an intermediate result to model context.
func (sm *Manager) ReplayEntries() []MessageEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.leafID == nil {
		return nil
	}
	ctx := buildSessionContext(sm.entries, *sm.leafID, sm.byIDs)
	traces := make(map[string][]MessageEntry)
	for _, entry := range walkPath(sm.entries, *sm.leafID, sm.byIDs) {
		if trace, ok := entry.(CodeTraceEntry); ok {
			id := trace.Run.ParentUseID
			traces[id] = append(traces[id], trace)
		}
	}
	if len(traces) == 0 {
		return ctx
	}
	out := make([]MessageEntry, 0, len(ctx))
	for _, entry := range ctx {
		out = append(out, entry)
		if msg, ok := entry.(SessionMessageEntry); ok && msg.Message.Role == llm.RoleAssistant {
			for _, call := range msg.Message.ToolCalls {
				out = append(out, traces[call.ID]...)
			}
		}
	}
	return out
}

// InterruptedCodeOperations reconstructs the last observed state of effects
// in an interrupted cell from the journal, independently of its JS output.
func InterruptedCodeOperations(entries []MessageEntry, parent string) string {
	var order []string
	latest := make(map[string]ToolRun)
	for _, entry := range entries {
		t, ok := entry.(CodeTraceEntry)
		if !ok || t.Run.ParentUseID != parent {
			continue
		}
		id := t.Run.ToolUseID
		if _, ok := latest[id]; !ok {
			order = append(order, id)
		}
		latest[id] = t.Run
	}
	if len(order) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(
		"The cell was interrupted. Completed effects remain applied; reconcile unknown operations before retrying.",
	)
	for _, id := range order {
		run := latest[id]
		status := run.Status.String()
		if run.Status == ToolInProgress || run.Status == ToolQueued {
			status = "outcome unknown"
		}
		fmt.Fprintf(&out, "\n%s %s: %s", id, run.Name, status)
	}
	return out.String()
}
