package agent

import (
	"encoding/json"
	"slices"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// ContextView is what the /context browser renders: the per-entry itemization
// of the current context plus the aggregate window/threshold numbers. It is
// numbers and previews only — a read-only projection, never an edit path.
type ContextView struct {
	session.ContextReport
	ContextWindow         int
	ContextTokens         int
	TokenSource           string // provider | estimate
	ThresholdTokens       int
	CompactionRecommended bool
}

// ContextReport builds the browser view for the current session.
func (engine *Engine) ContextReport() ContextView {
	stats := engine.contextStats()
	return ContextView{
		ContextReport:         engine.sessionRef().InspectContext(),
		ContextWindow:         stats.ContextWindow,
		ContextTokens:         stats.ContextTokens,
		TokenSource:           stats.TokenSource,
		ThresholdTokens:       stats.ThresholdTokens,
		CompactionRecommended: stats.CompactionRecommended,
	}
}

// TrimContextFrom drops everything before the entry from the model's context
// (append-only; see session.Manager.TrimContextFrom).
func (engine *Engine) TrimContextFrom(entryID string) error {
	return engine.sessionRef().TrimContextFrom(entryID)
}

// DropContextEntries deletes the given entries from the model's context
// (append-only; see session.Manager.DropContextEntries).
func (engine *Engine) DropContextEntries(ids []string) error {
	return engine.sessionRef().DropContextEntries(ids)
}

// contextStats snapshots quantitative context usage for the context tool.
// Tokens come from the newest provider-reported usage after the latest
// compaction. Until then the durable post-compaction estimate is authoritative;
// provider usage on messages retained from before compaction describes the old
// context and must not leak back into the counter. Numbers only — conversation
// content never leaves the engine through here.
func (engine *Engine) contextStats() tools.ContextStats {
	engine.mu.RLock()
	window := engine.contextWindow
	engine.mu.RUnlock()
	msgs := engine.sessionRef().BuildContext()
	entries := engine.sessionRef().PathEntries()
	usedBytes := estimateContextBytes(msgs)
	stats := tools.ContextStats{
		UsedBytes:     usedBytes,
		Messages:      len(msgs),
		ContextWindow: window,
		TokenSource:   "estimate",
		ContextTokens: usedBytes / 4,
	}
	usage, compactedTokens, unchangedSinceCompaction := currentContextUsage(entries, msgs)
	if usage.PromptTokens > 0 || usage.TotalTokens > 0 {
		stats.TokenSource = "provider"
		stats.ContextTokens = max(usage.PromptTokens, 0)
		if stats.ContextTokens == 0 {
			stats.ContextTokens = usage.TotalTokens
		}
	} else if unchangedSinceCompaction && compactedTokens > 0 {
		stats.ContextTokens = compactedTokens
	}
	if window > 0 {
		engine.mu.RLock()
		settings := engine.compactionSettings
		engine.mu.RUnlock()
		stats.ThresholdTokens = settings.ReminderThreshold(window)
		stats.CompactionRecommended = compaction.ShouldRemind(stats.ContextTokens, window, settings)
	}
	return stats
}

// currentContextUsage ignores provider counters attached to messages retained
// from before the latest compaction. PathEntries projects the latest compaction
// first, followed by retained ancestors and then descendants. Timestamps mark
// the new usage epoch even when a branch node sits between the compaction and
// the first projected message; the direct parent check also covers synthetic
// and legacy entries without timestamps.
func currentContextUsage(entries []session.MessageEntry, msgs []llm.Message) (llm.Usage, int, bool) {
	var latestCompaction session.CompactionEntry
	hasCompaction := false
	compactedTokens := 0
	afterCompaction := false
	postCompactionMessages := 0
	usage := llm.Usage{}
	for _, entry := range entries {
		switch typed := entry.(type) {
		case session.CompactionEntry:
			latestCompaction = typed
			hasCompaction = true
			compactedTokens = typed.Compaction.TokensAfter
			afterCompaction = false
			postCompactionMessages = 0
			usage = llm.Usage{}
		case session.SessionMessageEntry:
			if !hasCompaction {
				continue
			}
			if session.MessageFollowsCompaction(latestCompaction, typed) {
				afterCompaction = true
			}
			if !afterCompaction {
				continue
			}
			postCompactionMessages++
			if typed.Message.Role == llm.RoleAssistant &&
				(typed.Message.Usage.PromptTokens > 0 || typed.Message.Usage.TotalTokens > 0) {
				usage = typed.Message.Usage
			}
		}
	}
	if !hasCompaction {
		return lastReportedUsage(msgs), 0, false
	}
	return usage, compactedTokens, postCompactionMessages == 0
}

// estimateContextBytes is the JSON size of the model view — the measure both
// the byte counter and the token estimate derive from.
func estimateContextBytes(msgs []llm.Message) int {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return 0
	}
	return len(raw)
}

func estimateContextTokens(msgs []llm.Message) int {
	return estimateContextBytes(msgs) / 4
}

// lastReportedUsage returns the newest assistant usage in the model view.
func lastReportedUsage(msgs []llm.Message) llm.Usage {
	for i := range slices.Backward(msgs) {
		if msgs[i].Role != llm.RoleAssistant {
			continue
		}
		if usage := msgs[i].Usage; usage.PromptTokens > 0 || usage.TotalTokens > 0 {
			return usage
		}
	}
	return llm.Usage{}
}
