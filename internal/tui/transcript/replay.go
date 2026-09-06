package transcript

import (
	"encoding/json"
	"strings"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// ReplaySnapshot builds a transcript snapshot from persisted session entries
// (user/assistant text; tool rows simplified away). It is the load-side
// counterpart of the Mapper: the same projection rules, applied to a whole
// history at once instead of event by event.
func ReplaySnapshot(entries []session.MessageEntry) session.Snapshot {
	var snap session.Snapshot
	// Sub-agent titles, keyed by the spawn call that made them. The history
	// already carries what the row needs to be named, so the outcome row can
	// say role(description) without asking the job store anything.
	spawnTitles := make(map[string]string)
	var pendingCompaction *session.CompactionEntry
	emitCompaction := func() {
		if pendingCompaction == nil {
			return
		}
		snap = session.Apply(snap, session.CompactionComplete{
			ID:         pendingCompaction.ID,
			Compaction: pendingCompaction.Compaction,
		})
		pendingCompaction = nil
	}
	for _, entry := range entries {
		switch entry.GetType() {
		case session.EntryCompaction:
			compacted := entry.(session.CompactionEntry)
			pendingCompaction = &compacted
		case session.EntryMessage:
			messageEntry := entry.(session.SessionMessageEntry)
			if pendingCompaction != nil && session.MessageFollowsCompaction(*pendingCompaction, messageEntry) {
				emitCompaction()
			}
			msg := messageEntry.Message
			switch msg.Role {
			case llm.RoleUser:
				// A delivered child outcome is a receipt, not something the
				// user typed. It becomes the sub-agent row whose spawn call
				// the projection dropped, so a resumed session still shows
				// what the child came back with.
				if outcome, ok := parseOutcomeDelivery(messageEntry); ok {
					snap = session.Apply(snap, replayedOutcome(entry.GetID(), outcome, spawnTitles))
					continue
				}
				// Recall blocks are prepended by the turn, not typed by the
				// user; a replayed transcript shows the prompt as it was sent.
				snap = session.Apply(snap, session.UserAppend{
					ID:   entry.GetID(),
					Text: memory.StripReminders(msg.Content),
				})
			case llm.RoleAssistant:
				for _, call := range msg.ToolCalls {
					if strings.EqualFold(call.Function.Name, "agent_spawn") {
						spawnTitles[call.ID] = tools.SpawnTitleFromInput(
							json.RawMessage(call.Function.Arguments),
						)
					}
				}
				text := msg.Content
				var blocks []session.ContentBlock
				if strings.TrimSpace(msg.ReasoningContent) != "" {
					blocks = append(
						blocks,
						session.ContentBlock{Type: session.BlockThinking, Text: msg.ReasoningContent},
					)
				}
				if text != "" {
					blocks = append(blocks, session.ContentBlock{Type: session.BlockText, Text: text})
				}
				snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
					ID:      entry.GetID(),
					State:   session.StateComplete,
					Text:    text,
					Content: blocks,
					Usage: session.TokenUsage{
						PromptTokens:     msg.Usage.PromptTokens,
						CompletionTokens: msg.Usage.CompletionTokens,
						CachedTokens:     msg.Usage.CachedTokens(),
						TotalTokens:      msg.Usage.TotalTokens,
					},
				}})
			}
		}
	}
	emitCompaction()
	return snap
}

// outcomeDeliverySuffix marks a message the engine appended as the receipt of
// a terminal child assignment. It is host metadata: no tool output can forge
// it, so a message carrying it is a child outcome and nothing else.
const outcomeDeliverySuffix = ":terminal"

// parseOutcomeDelivery reads the outcome out of a delivery receipt. The body
// is the JSON the parent was handed, wrapped in a system reminder; anything
// that does not parse into a terminal outcome is left to render as it always
// did rather than disappearing.
func parseOutcomeDelivery(entry session.SessionMessageEntry) (job.Outcome, bool) {
	if !strings.HasSuffix(entry.DeliveryID, outcomeDeliverySuffix) {
		return job.Outcome{}, false
	}
	body := entry.Message.Content
	start := strings.Index(body, "{")
	end := strings.LastIndex(body, "}")
	if start < 0 || end <= start {
		return job.Outcome{}, false
	}
	var outcome job.Outcome
	if err := json.Unmarshal([]byte(body[start:end+1]), &outcome); err != nil {
		return job.Outcome{}, false
	}
	if outcome.JobID == "" || !outcome.Status.Terminal() {
		return job.Outcome{}, false
	}
	return outcome, true
}

// replayedOutcome turns a delivered outcome into the local row that stands in
// for the spawn call the projection dropped: named after the child when the
// history still holds its spawn arguments, after the job id when it does not.
func replayedOutcome(id string, outcome job.Outcome, spawnTitles map[string]string) session.ChildOutcome {
	title := strings.TrimSpace(spawnTitles[outcome.ParentToolUseID])
	if title == "" {
		title = outcome.JobID
	}
	summary := strings.TrimSpace(outcome.Summary)
	if summary == "" {
		summary = strings.TrimSpace(outcome.Error)
	}
	return session.ChildOutcome{
		ID:      id,
		Title:   title,
		Summary: summary,
		Status:  outcomeToolStatus(outcome.Status),
	}
}

// outcomeToolStatus is how a finished child's job status reads as a tool row.
func outcomeToolStatus(s job.Status) session.ToolStatus {
	switch s {
	case job.StatusFailed, job.StatusTimedOut:
		return session.ToolError
	case job.StatusCancelled:
		return session.ToolCancelled
	default:
		return session.ToolDone
	}
}
