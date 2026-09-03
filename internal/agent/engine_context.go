package agent

import (
	"encoding/json"
	"maps"
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
	TokenSource           string // provider | calibrated | estimate
	ThresholdTokens       int
	CompactionRecommended bool
	MicroElidedResults    int
	MicroElidedBytes      int
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
		MicroElidedResults:    stats.MicroElidedResults,
		MicroElidedBytes:      stats.MicroElidedBytes,
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
// Tokens come from the live calibration: the provider's count for the last
// request plus the estimated change since, so a tool result appended after
// that response is already in the number the pressure ladder reads. Without
// an observation — a fresh resume, a compaction, a model switch — the session
// log's own provider counters after the latest compaction answer instead, then
// the durable post-compaction estimate; provider usage on messages retained
// from before compaction describes the old context and must not leak back into
// the counter. Numbers only — conversation content never leaves the engine
// through here.
func (engine *Engine) contextStats() tools.ContextStats {
	engine.mu.RLock()
	window := engine.contextWindow
	engine.mu.RUnlock()
	msgs, micro := engine.providerContext(engine.sessionRef())
	usedBytes := estimateContextBytes(msgs)
	tokens, source := engine.calibratedTokens(usedBytes / 4)
	stats := tools.ContextStats{
		UsedBytes:          usedBytes,
		Messages:           len(msgs),
		ContextWindow:      window,
		TokenSource:        source,
		ContextTokens:      tokens,
		MicroElidedResults: micro.Results,
		MicroElidedBytes:   micro.BytesElided,
	}
	if source == "estimate" {
		entries := engine.sessionRef().PathEntries()
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

// providerContext returns what the provider sees of the session: the durable
// context with old oversized tool results microcompacted. Results of the
// current round — everything after the last assistant message — always ride
// verbatim; older ones are stubbed once the projection estimates past the
// compact-advice threshold, and the stub set then stays frozen on the engine
// so the cached prompt prefix survives the next round. The request path and
// contextStats share this projection, so pressure advice and /context
// describe the context the next request will carry.
func (engine *Engine) providerContext(sess *Session) ([]llm.Message, compaction.MicroReport) {
	msgs := sess.BuildContext()
	offset := engine.tokenOffset()
	engine.mu.RLock()
	window := engine.contextWindow
	settings := engine.compactionSettings
	frozen := maps.Clone(engine.microStubbed)
	engine.mu.RUnlock()

	trigger := settings.ReminderThreshold(window)
	// A tenth of the window of headroom below the trigger: the set is
	// extended in batches, not one result per round.
	target := max(trigger-window/10, 0)
	// The thresholds are provider-space numbers, but Microcompact weighs its
	// candidates with the per-message JSON estimate and re-weighs after every
	// stub it applies — the estimate is not ours to shift from out here. So
	// the calibration moves the thresholds down by the measured overhead
	// instead: a prompt whose system text and tool schemas cost 20k tokens
	// hits the real trigger 20k of estimate early. A trigger of 0 means
	// "never extend the frozen set" and must stay 0 through the shift.
	if trigger > 0 {
		trigger = max(trigger-offset, 1)
	}
	target = max(target-offset, 0)
	projected, report, stubbed := compaction.Microcompact(msgs, compaction.MicroPolicy{
		KeepVerbatim:     keepToolResultVerbatim,
		Advice:           microAdvice,
		KeepRecentTokens: settings.KeepRecentTokens(),
		TriggerTokens:    trigger,
		TargetTokens:     target,
	}, frozen)
	engine.mu.Lock()
	engine.microStubbed = stubbed
	engine.mu.Unlock()
	return projected, report
}

// keepToolResultVerbatim reports whether a tool result must survive
// provider-view microcompaction intact. Two kinds do: anchor carriers, whose
// LINE#HASH lines the hashline edit protocol consumes — a stubbed anchor read
// can no longer authorize its edit — and results no re-run can reproduce, an
// answer the user typed or a sub-agent's final report. args is the raw JSON
// string the model sent; unparseable args fail closed and keep the result.
func keepToolResultVerbatim(tool, args string) bool {
	switch tool {
	case "grep", "question", "agent_wait":
		return true
	case "read":
		var call struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal([]byte(args), &call) != nil {
			return true
		}
		return call.Mode == "edit"
	default:
		return false
	}
}

// microAdvice returns the recovery sentence a stub carries for its tool. A
// bash command or an MCP call may have changed the world, so "run it again"
// is not advice the model can follow blindly; every other tool falls through
// to the package's generic re-run sentence.
func microAdvice(tool, _ string) string {
	switch tool {
	case "bash", "mcp_call":
		return "Its output is gone from context; re-run it only if the call is read-only, " +
			"otherwise work from what you already concluded."
	default:
		return ""
	}
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

// tokenObservation pairs the estimate of the projection one request carried
// with the prompt tokens the provider counted for it. The difference is the
// prompt overhead (system prompt, tool schemas) plus the estimate's scale
// error; both ride along unchanged until the context changes shape.
type tokenObservation struct {
	estimate int // estimateContextTokens of the projection sent
	prompt   int // usage.PromptTokens the provider reported for it
}

// noteTokenObservation records what one request actually cost: the estimate of
// the projection that was sent next to the provider's own count for it. A
// provider that reports no usage leaves the previous observation standing — a
// calibration one round old beats none at all.
func (engine *Engine) noteTokenObservation(sentEstimate int, usage llm.Usage) {
	if usage.PromptTokens <= 0 {
		return
	}
	engine.mu.Lock()
	engine.tokenObs = &tokenObservation{estimate: sentEstimate, prompt: usage.PromptTokens}
	engine.mu.Unlock()
}

// calibratedTokens turns a raw estimate of the current projection into the
// best available count: the provider's last prompt count plus the estimated
// change since that request. Source is "provider" when nothing changed since,
// "calibrated" when the estimate had to fill in a delta, "estimate" without
// an observation.
func (engine *Engine) calibratedTokens(estimate int) (tokens int, source string) {
	engine.mu.RLock()
	obs := engine.tokenObs
	engine.mu.RUnlock()
	if obs == nil {
		return estimate, "estimate"
	}
	if delta := estimate - obs.estimate; delta != 0 {
		return max(obs.prompt+delta, 0), "calibrated"
	}
	return max(obs.prompt, 0), "provider"
}

// tokenOffset is the prompt overhead the last observation measured — what the
// provider counted beyond the estimate of the very same projection. It
// converts a provider-space number into estimate space; 0 without an
// observation leaves such a number where it was.
func (engine *Engine) tokenOffset() int {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if engine.tokenObs == nil {
		return 0
	}
	return engine.tokenObs.prompt - engine.tokenObs.estimate
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
