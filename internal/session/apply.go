package session

import (
	"fmt"
	"maps"
	"slices"
)

// Apply returns a new snapshot with ev applied (immutable reducer).
func Apply(s Snapshot, ev Event) Snapshot {
	out := cloneSnapshot(s)
	applyInPlace(&out, ev)
	return out
}

// Reducer owns a session snapshot and applies events without copying the full
// history. NewReducer and Replace clone the top-level collections so callers
// cannot observe in-place updates through the snapshot they supplied.
type Reducer struct {
	snapshot Snapshot
}

// NewReducer builds an owned mutable reducer from s.
func NewReducer(s Snapshot) *Reducer {
	return &Reducer{snapshot: cloneSnapshot(s)}
}

// Snapshot returns the reducer's current state for read-only use.
func (r *Reducer) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{}
	}
	return r.snapshot
}

// Replace resets the reducer to an owned copy of s.
func (r *Reducer) Replace(s Snapshot) {
	if r != nil {
		r.snapshot = cloneSnapshot(s)
	}
}

// Apply mutates the reducer's owned snapshot with ev.
func (r *Reducer) Apply(ev Event) {
	if r != nil {
		applyInPlace(&r.snapshot, ev)
	}
}

func cloneSnapshot(s Snapshot) Snapshot {
	return Snapshot{
		Messages:   append([]Message(nil), s.Messages...),
		Tools:      maps.Clone(s.Tools),
		Compacting: s.Compacting,
	}
}

func applyInPlace(out *Snapshot, ev Event) {
	switch e := ev.(type) {
	case UserAppend:
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("user-%d", len(out.Messages)+1)
		}
		out.Messages = append(out.Messages, Message{
			ID:     id,
			Role:   RoleUser,
			Text:   e.Text,
			Queued: e.Queued,
		})
	case UserPromoted:
		for i := range slices.Backward(out.Messages) {
			if out.Messages[i].ID == e.ID && out.Messages[i].Role == RoleUser {
				out.Messages[i].Queued = false
				break
			}
		}
	case LocalBashStart:
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("bash-%d", len(out.Messages)+1)
		}
		out.Messages = append(out.Messages, Message{
			ID:   id,
			Role: RoleLocalBash,
			Text: e.Command,
		})
		if out.Tools == nil {
			out.Tools = make(map[string]ToolRun)
		}
		out.Tools[id] = ToolRun{
			ToolUseID: id,
			Name:      "bash",
			Status:    ToolInProgress,
			Detail:    e.Command,
			Local:     true,
		}
	case WatchFired:
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("watch-%d", len(out.Messages)+1)
		}
		out.Messages = append(out.Messages, Message{
			ID:   id,
			Role: RoleWatch,
			Text: e.Label,
		})
		if out.Tools == nil {
			out.Tools = make(map[string]ToolRun)
		}
		out.Tools[id] = ToolRun{
			ToolUseID: id,
			Name:      "watch",
			Status:    ToolDone,
			Detail:    e.Label,
			Output:    e.Text,
			Local:     true,
		}
	case PlanActionRan:
		// One UI-only row per executed action: the durable record rides in the
		// plan snapshot, so this row is what makes a failed run visible even
		// when the transition it blocked never lands.
		id := fmt.Sprintf("plan-action-%d", len(out.Messages)+1)
		status := ToolDone
		if e.Status == PlanActionRunFailed {
			status = ToolError
		}
		detail := fmt.Sprintf("%s@%s → %s", e.Type, e.Event, e.Status)
		out.Messages = append(out.Messages, Message{ID: id, Role: RolePlan, Text: detail})
		if out.Tools == nil {
			out.Tools = make(map[string]ToolRun)
		}
		out.Tools[id] = ToolRun{
			ToolUseID: id,
			Name:      planActionToolName,
			Status:    status,
			Detail:    detail,
			Output:    e.Error,
			Local:     true,
		}
	case AssistantMessageUpdate:
		m := e.Message
		m.Role = RoleAssistant
		if m.Text == "" {
			m.Text = m.FlatText()
		}
		if i, ok := assistantReplaceIndex(out.Messages, m); ok {
			if m.ID == "" {
				m.ID = out.Messages[i].ID
			}
			// Keep last known usage when a streaming delta omits it.
			if !m.Usage.Reported() && out.Messages[i].Usage.Reported() {
				m.Usage = out.Messages[i].Usage
			}
			// Same for turn metadata: a terminal update that omits model/start
			// keeps what streaming events established.
			prev := out.Messages[i]
			if m.Model == "" {
				m.Model = prev.Model
			}
			if m.Started.IsZero() {
				m.Started = prev.Started
			}
			if m.Ended.IsZero() {
				m.Ended = prev.Ended
			}
			out.Messages[i] = m
		} else {
			if m.ID == "" {
				m.ID = fmt.Sprintf("assistant-%d", len(out.Messages)+1)
			}
			out.Messages = append(out.Messages, m)
		}
		for _, b := range m.Content {
			if b.Type != BlockToolUse || b.ID == "" {
				continue
			}
			if _, exists := out.Tools[b.ID]; exists {
				continue
			}
			if out.Tools == nil {
				out.Tools = make(map[string]ToolRun)
			}
			out.Tools[b.ID] = ToolRun{
				ToolUseID: b.ID,
				Name:      b.Name,
				Status:    ToolInProgress,
				Detail:    b.Input,
			}
		}
	case ToolData:
		if out.Tools == nil {
			out.Tools = make(map[string]ToolRun)
		}
		run := e.Run
		if prev, ok := out.Tools[run.ToolUseID]; ok {
			if run.Name == "" {
				run.Name = prev.Name
			}
			if run.Detail == "" {
				run.Detail = prev.Detail
			}
			if run.Status == ToolInProgress && run.Output == "" && prev.Output != "" {
				run.Output = prev.Output
			}
			if prev.Local {
				run.Local = true
			}
		}
		out.Tools[run.ToolUseID] = run
	case CancelStreaming:
		if i := lastAssistantIndex(out.Messages); i >= 0 && out.Messages[i].State == StateStreaming {
			out.Messages[i].State = StateCancelled
		}
		for id, run := range out.Tools {
			if run.Local {
				continue // Esc during agent must not cancel user "!cmd" bash
			}
			switch run.Status {
			case ToolInProgress, ToolQueued:
				run.Status = ToolCancelled
				out.Tools[id] = run
			}
		}
		out.Compacting = false
	case CompactionStarted:
		out.Compacting = true
	case CompactionComplete:
		out.Compacting = false
		if !e.Failed {
			id := e.ID
			if id == "" {
				id = fmt.Sprintf("compaction-%d", len(out.Messages)+1)
			}
			usage := TokenUsage{}
			if e.Compaction.TokensAfter > 0 {
				usage = TokenUsage{
					PromptTokens: e.Compaction.TokensAfter,
					TotalTokens:  e.Compaction.TokensAfter,
					Estimated:    true,
				}
			}
			out.Messages = append(out.Messages, Message{
				ID:      id,
				Role:    RoleCompaction,
				Text:    e.Compaction.Report(),
				Summary: e.Compaction.Summary,
				Usage:   usage,
			})
		}
	}
}

// assistantReplaceIndex finds the assistant row to replace for message-update.
// The in-flight (streaming) turn always absorbs its updates, even when a
// queued user message was appended below it — submit-while-streaming inserts
// the user row behind the running turn. When no turn is streaming, the update
// replaces the last assistant with the same ID; otherwise it is a new turn.
func assistantReplaceIndex(msgs []Message, update Message) (int, bool) {
	for i := range slices.Backward(msgs) {
		if msgs[i].Role == RoleAssistant && msgs[i].State == StateStreaming {
			return i, true
		}
	}
	if last := lastAssistantIndex(msgs); last >= 0 && update.ID != "" && update.ID == msgs[last].ID {
		return last, true
	}
	return -1, false
}

func lastAssistantIndex(msgs []Message) int {
	for i := range slices.Backward(msgs) {
		if msgs[i].Role == RoleAssistant {
			return i
		}
	}
	return -1
}

// IsStreaming reports whether inference, tools, or compaction are still active.
func IsStreaming(s Snapshot) bool {
	if s.Compacting {
		return true
	}
	if i := lastAssistantIndex(s.Messages); i >= 0 && s.Messages[i].State == StateStreaming {
		return true
	}
	return HasRunningTools(s)
}

// StreamingModel returns the model id of the assistant round currently
// streaming — the model answering right now — or "" when no round streams
// or the provider has not named it yet.
func StreamingModel(s Snapshot) string {
	if i := lastAssistantIndex(s.Messages); i >= 0 && s.Messages[i].State == StateStreaming {
		return s.Messages[i].Model
	}
	return ""
}

// HasRunningTools reports in-progress or queued agent tool runs
// (excludes user-initiated local bash).
func HasRunningTools(s Snapshot) bool {
	for _, run := range s.Tools {
		if run.Local {
			continue
		}
		switch run.Status {
		case ToolInProgress, ToolQueued:
			return true
		}
	}
	return false
}

// RunningToolCount returns how many agent tools are in-progress/queued.
func RunningToolCount(s Snapshot) int {
	n := 0
	for _, run := range s.Tools {
		if run.Local {
			continue
		}
		switch run.Status {
		case ToolInProgress, ToolQueued:
			n++
		}
	}
	return n
}

// LastAssistant returns the last assistant message, if any.
func LastAssistant(s Snapshot) (Message, bool) {
	i := lastAssistantIndex(s.Messages)
	if i < 0 {
		return Message{}, false
	}
	return s.Messages[i], true
}
