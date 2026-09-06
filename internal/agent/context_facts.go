package agent

import (
	"hash/fnv"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/session"
)

// ContextObservation projects this engine's context layer into the
// diagnostics DTO: the window it budgets against, what occupies it, the
// compaction policy and pressure, and what the last system-prompt render was
// assembled from.
//
// Every number here is read from work that already happened. The usage is the
// same projection the context tool reports, the compaction counters are a
// walk of entries the session manager already holds, and the prompt metadata
// comes from the render record rather than from a render: building the prompt
// again would re-read the instruction files and the skill catalog from disk,
// and the memory store would re-scan its directory — an observation is not a
// reason to load anything.
//
// Nothing that carries text comes through. The compaction summary, the
// context entries and their previews, the prompt itself and every memory stay
// with their owners; what crosses is counts, sizes and provenance.
func (engine *Engine) ContextObservation() diag.ContextState {
	if engine == nil {
		return diag.ContextState{}
	}
	// contextStats takes its own locks and writes the frozen microcompaction
	// set through providerContext, so it is called before engine.mu is held
	// here — and only ever through the owner's own measurement, so the view
	// the harness reports is the view the next request would carry.
	stats := engine.contextStats()
	compactions := engine.sessionRef().CompactionStats()

	engine.mu.RLock()
	defer engine.mu.RUnlock()

	settings := engine.compactionSettings
	window := engine.contextWindow
	state := diag.ContextState{
		Known: true,
		Window: diag.ContextWindow{
			Model:     engine.modelCfg.ContextWindow,
			Ceiling:   engine.contextCeiling,
			Override:  engine.contextOverride,
			Effective: window,
		},
		Usage: diag.ContextUsage{
			Tokens:             stats.ContextTokens,
			TokenSource:        stats.TokenSource,
			Bytes:              stats.UsedBytes,
			Messages:           stats.Messages,
			MicroElidedResults: stats.MicroElidedResults,
			MicroElidedBytes:   stats.MicroElidedBytes,
		},
		Compaction: diag.ContextCompaction{
			Enabled:        settings.Enabled(),
			ReminderTokens: settings.ReminderTokens(),
			Threshold:      stats.ThresholdTokens,
			Headroom:       settings.ReverseTokens(),
			KeepRecent:     settings.KeepRecentTokens(),
			Recommended:    stats.CompactionRecommended,
			Pending:        engine.pendingCompact,
			Strikes:        engine.compactStrikes,
			Stopped:        engine.compactStopped,
			Count:          compactions.Count,
			Last:           lastCompactionFacts(compactions),
		},
		Sources: promptSourceFacts(engine.promptRec, engine.memory != nil),
	}
	state.Revision = contextRevision(engine.promptRec.renders, state)
	return state
}

// lastCompactionFacts copies the latest compaction's counters. Known travels
// with them: a compaction that freed nothing and no compaction at all are
// both zeroes, and only one of them is a fact about this session's history.
func lastCompactionFacts(stats session.CompactionStats) diag.ContextCompactionLast {
	return diag.ContextCompactionLast{
		Known:              stats.Known,
		FromTrim:           stats.FromTrim,
		TokensBefore:       stats.TokensBefore,
		TokensAfter:        stats.TokensAfter,
		MessagesSummarized: stats.MessagesSummarized,
		MessagesKept:       stats.MessagesKept,
	}
}

// promptSourceFacts copies the last render's record into the DTO. Before the
// first render there is no record, and Loaded says so rather than letting a
// prompt nobody has built yet read as one that loaded nothing.
//
// The instruction scopes are copied because the record's slice is the
// engine's; a snapshot that shared it would go on changing after it was
// taken.
func promptSourceFacts(rec promptRecord, store bool) diag.ContextSources {
	if rec.renders == 0 {
		return diag.ContextSources{MemoryStore: store}
	}
	scopes := make([]string, len(rec.prompt.InstructionScopes))
	copy(scopes, rec.prompt.InstructionScopes)
	return diag.ContextSources{
		Loaded:            true,
		PromptBytes:       rec.bytes,
		Instructions:      rec.prompt.Instructions,
		InstructionScopes: scopes,
		SkillDir:          rec.prompt.SkillDir,
		Skills:            rec.prompt.Skills,
		MemoryStore:       store,
		MemoryFacts:       rec.memory.Facts,
		MemoryStanding:    rec.memory.Standing,
		MemoryRunes:       rec.memory.Runes,
	}
}

// contextRevision fingerprints the generation an observation belongs to: the
// prompt render behind it, the window it budgets against, and the compactions
// and loaded inputs that shaped it. Usage drifts with every appended message
// and is deliberately left out — a reader comparing two snapshots wants to
// know whether the ground moved, not whether the conversation grew. It
// digests numbers only.
func contextRevision(renders uint64, state diag.ContextState) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(strconv.FormatUint(renders, 10)))
	for _, n := range []int{
		state.Window.Effective,
		state.Compaction.Count,
		state.Sources.PromptBytes,
		state.Sources.Instructions,
		state.Sources.Skills,
		state.Sources.MemoryFacts,
	} {
		_, _ = digest.Write([]byte{0x1f})
		_, _ = digest.Write([]byte(strconv.Itoa(n)))
	}
	return strconv.FormatUint(digest.Sum64(), 16)
}
