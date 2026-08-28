package transcript

import (
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/session"
)

// ReplaySnapshot builds a transcript snapshot from persisted session entries
// (user/assistant text; tool rows simplified away). It is the load-side
// counterpart of the Mapper: the same projection rules, applied to a whole
// history at once instead of event by event.
func ReplaySnapshot(entries []session.MessageEntry) session.Snapshot {
	var snap session.Snapshot
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
				// Recall blocks are prepended by the turn, not typed by the
				// user; a replayed transcript shows the prompt as it was sent.
				snap = session.Apply(snap, session.UserAppend{
					ID:   entry.GetID(),
					Text: memory.StripReminders(msg.Content),
				})
			case llm.RoleAssistant:
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
