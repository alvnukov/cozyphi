package agent

import (
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

// compactAdviceReason names who asked for the nudge, so the model can weigh
// the instruction: a plan compact action placed the marker deliberately, real
// context pressure is a fact about the session.
const (
	compactAdviceFromPlan     = "a plan compact action fired"
	compactAdviceFromPressure = "context pressure"
)

// compactAdviceReminder renders the compaction recommendation the engine
// prepends to the next user prompt. It uses the same <system-reminder> wire
// format memory recall and watch events use, so a resumed transcript strips
// it back out. usage and window may be 0 when the trigger has no numbers.
func compactAdviceReminder(reason string, usage, window int) string {
	var b strings.Builder
	b.WriteString(reminderOpen + "\n")
	if window > 0 && usage > 0 {
		fmt.Fprintf(&b, "Context pressure: ~%d of %d context tokens (~%d%%). ", usage, window, usage*100/window)
	}
	fmt.Fprintf(&b, "%s recommends compacting the context now.\n", reason)
	b.WriteString("Do not compact mid tool-sequence. At the next safe boundary:\n")
	b.WriteString(
		"1. Record what must survive compaction — current hypothesis, file anchors, open risks, running command ids — in the durable plan's workingContext or session notes.\n",
	)
	b.WriteString("2. Call the context tool with {\"action\":\"compact\"} yourself.\n")
	b.WriteString("Then continue the turn's work; do not end the turn on this reminder.\n")
	b.WriteString(reminderClose)
	return b.String()
}

// queueCompactAdvice parks a rendered recommendation; it rides exactly one
// prompt. The first reason wins — a plan action and pressure in the same
// breath produce one reminder, not two.
func (engine *Engine) queueCompactAdvice(reason string, usage, window int) {
	if reason == "" {
		return
	}
	engine.mu.Lock()
	if engine.compactAdvice == "" {
		engine.compactAdvice = compactAdviceReminder(reason, usage, window)
	}
	engine.mu.Unlock()
}

// drainCompactAdvice takes the parked recommendation, if any.
func (engine *Engine) drainCompactAdvice() string {
	engine.mu.Lock()
	reminder := engine.compactAdvice
	engine.compactAdvice = ""
	engine.mu.Unlock()
	return reminder
}

// SetCompactionSettings swaps the compaction policy — the reminder threshold
// above all. Callers hold no engine lock; the next pressure check reads it.
func (engine *Engine) SetCompactionSettings(s compaction.Settings) {
	engine.mu.Lock()
	engine.compactionSettings = s
	engine.mu.Unlock()
}

// noteCompactPressure runs at turn end: real context pressure — measured
// from the session (contextStats resolves provider-reported or estimated
// tokens; some providers report none, which is why the old auto-compact
// trigger never fired) — queues the compact advice for the next prompt.
// One reminder per crossing: compactAdvised latches until a compaction
// lands or usage falls back under the threshold.
func (engine *Engine) noteCompactPressure() {
	stats := engine.contextStats()
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if !compaction.ShouldRemind(stats.ContextTokens, stats.ContextWindow, engine.compactionSettings) {
		engine.compactAdvised = false // back under the threshold: re-arm
		return
	}
	if engine.compactAdvised {
		return
	}
	engine.compactAdvised = true
	if engine.compactAdvice == "" {
		engine.compactAdvice = compactAdviceReminder(
			compactAdviceFromPressure,
			stats.ContextTokens,
			stats.ContextWindow,
		)
	}
}

// rearmCompactAdvice lets the next pressure crossing advise again; called
// after a compaction entry lands.
func (engine *Engine) rearmCompactAdvice() {
	engine.mu.Lock()
	engine.compactAdvised = false
	engine.mu.Unlock()
}
