package agent

import (
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

// compactAdviceReason names who asked for the nudge, so the model can weigh
// the instruction: a plan compact action placed the marker deliberately, real
// context pressure is a fact about the session.
const (
	compactAdviceFromPlan     = "a plan compact action fired"
	compactAdviceFromPressure = "context pressure"
)

// The pressure escalation ladder, in agent turns that ended over the reminder
// threshold without a compaction landing: soft reminders repeat every turn;
// at compactStrikesHard the executor refuses every tool but the context tool;
// one more uncompacted turn stops the model loop entirely.
const (
	compactStrikesHard = 3
	compactStrikesStop = 4
)

// compactGateAllows is the one tool that keeps running in hard mode: the
// model must always be able to compact its way out.
const compactGateAllows = "context"

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
		"1. Record what must survive compaction — current hypothesis, file anchors, open risks, running command ids — in your last assistant message: recent messages survive compaction verbatim.\n",
	)
	b.WriteString(
		"2. Tell the user in one short line — the pressure numbers and that you are compacting; silent compliance reads as an ignored message.\n",
	)
	b.WriteString("3. Call the context tool with {\"action\":\"compact\"} yourself.\n")
	b.WriteString("Then continue the turn's work; do not end the turn on this reminder.\n")
	b.WriteString(reminderClose)
	return b.String()
}

// compactPressureReminder renders the pressure reminder for the given strike
// count: the soft checklist while the model still picks its own moment, and a
// hard directive from the strike the executor starts blocking tools.
func compactPressureReminder(strikes, usage, window int) string {
	if strikes < compactStrikesHard {
		return compactAdviceReminder(compactAdviceFromPressure, usage, window)
	}
	var b strings.Builder
	b.WriteString(reminderOpen + "\n")
	if window > 0 && usage > 0 {
		fmt.Fprintf(&b, "Context limit: ~%d of %d context tokens (~%d%%). ", usage, window, usage*100/window)
	}
	fmt.Fprintf(&b, "This is reminder %d: the context has not been compacted. ", strikes)
	b.WriteString("Every tool except the context tool is now blocked.\n")
	b.WriteString("Tell the user in one short line — context limit reached, tools blocked, compacting now.\n")
	b.WriteString("You MUST call the context tool with {\"action\":\"compact\"} now, before any other work.\n")
	b.WriteString("Do not end the turn on this reminder.\n")
	b.WriteString(reminderClose)
	return b.String()
}

// compactGateDirective is the refusal the executor returns in hard mode.
func compactGateDirective() string {
	return "context limit reached: compact the context first — call the context tool with " +
		"{\"action\":\"compact\"}; every other tool is blocked until the context is compacted"
}

// compactPressureNoticeLabel renders the transcript row for one strike: the
// same numbers the reminder carries, plus where the ladder stands.
func compactPressureNoticeLabel(strikes, usage, window int) string {
	if window <= 0 || usage <= 0 {
		return fmt.Sprintf("context pressure · reminder %d of %d", strikes, compactStrikesStop)
	}
	if strikes >= compactStrikesHard {
		return fmt.Sprintf(
			"context limit ~%d of %d tokens (~%d%%) · reminder %d of %d — every tool but context blocked",
			usage, window, usage*100/window, strikes, compactStrikesStop)
	}
	return fmt.Sprintf(
		"context pressure ~%d of %d tokens (~%d%%) · reminder %d of %d",
		usage, window, usage*100/window, strikes, compactStrikesStop)
}

// emitCompactNotice publishes the user-facing row for a reminder the model
// is also told about. A nil sink (tests, headless runs) drops it silently.
func (engine *Engine) emitCompactNotice(label string, hard bool) {
	if sink := engine.sessionEvents; sink != nil {
		sink(session.CompactNotice{Label: label, Hard: hard})
	}
}

// queuePlanCompactAdvice parks the plan compact action's recommendation; it
// rides exactly one prompt. The first reason wins — a plan action and pressure
// in the same breath produce one reminder, not two (pressure supersedes at
// turn end). Pressure never queues here: it takes the ladder directly.
func (engine *Engine) queuePlanCompactAdvice() {
	engine.mu.Lock()
	parked := false
	if engine.compactAdvice == "" {
		engine.compactAdvice = compactAdviceReminder(compactAdviceFromPlan, 0, 0)
		parked = true
	}
	engine.mu.Unlock()
	// One transcript row per delivered reminder: the user sees the same
	// nudge the model got.
	if parked {
		engine.emitCompactNotice(compactAdviceFromPlan+" — compacting recommended", false)
	}
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
// trigger never fired) — escalates the reminder ladder. Every agent turn
// that ends over the threshold without a compaction landing strikes once and
// re-queues the reminder, so ignoring it cannot buy silence; falling back
// under the threshold or landing a compaction resets the ladder.
func (engine *Engine) noteCompactPressure() {
	stats := engine.contextStats()
	engine.mu.Lock()
	if !compaction.ShouldRemind(stats.ContextTokens, stats.ContextWindow, engine.compactionSettings) {
		engine.compactStrikes = 0 // back under the threshold: forgive
		engine.mu.Unlock()
		return
	}
	engine.compactStrikes++
	// Pressure is a fresh fact about the session and supersedes whatever a
	// plan compact action parked earlier in the turn — the plan nudge must
	// not mask the numbers.
	engine.compactAdvice = compactPressureReminder(engine.compactStrikes, stats.ContextTokens, stats.ContextWindow)
	if engine.compactStrikes >= compactStrikesStop {
		engine.compactStopped = true
	}
	hard := engine.compactStrikes >= compactStrikesHard
	label := compactPressureNoticeLabel(engine.compactStrikes, stats.ContextTokens, stats.ContextWindow)
	engine.mu.Unlock()
	// Emitted outside the lock: the sink crosses into the controller, which
	// must never wait on the engine mutex.
	engine.emitCompactNotice(label, hard)
}

// compactHardMode reports whether the reminder ladder has passed the hard
// strike count: the executor must refuse every tool but the context tool.
func (engine *Engine) compactHardMode() bool {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.compactStrikes >= compactStrikesHard
}

// compactStopActive reports the full stop: the model ignored even the hard
// directive, so Loop refuses to run until a compaction lands.
func (engine *Engine) compactStopActive() bool {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.compactStopped
}

// compactGateFor is the executor's hard-compaction gate: in hard mode every
// tool but the context tool is refused with the directive; an empty string
// lets the call through.
func (engine *Engine) compactGateFor(tool string) string {
	if !engine.compactHardMode() || tool == compactGateAllows {
		return ""
	}
	return compactGateDirective()
}

// rearmCompactAdvice resets the escalation ladder — strikes, hard mode and
// the full stop — after a compaction entry lands.
func (engine *Engine) rearmCompactAdvice() {
	engine.mu.Lock()
	engine.compactStrikes = 0
	engine.compactStopped = false
	engine.mu.Unlock()
}
