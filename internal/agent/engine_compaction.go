package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	llmclient "github.com/alvnukov/cozyphi/internal/llm/client"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

// compactForOverflow compacts the session after a provider context-overflow
// rejection. It reports whether anything was summarized: a false result means
// the caller must surface the original error instead of retrying the same
// oversized request.
func (engine *Engine) compactForOverflow(
	ctx context.Context,
	yield func(session.Event, error) bool,
	rt roundRuntime,
) (bool, error) {
	prep, err := compaction.PrepareCompact(engine.sessionRef().PathEntries(), compaction.DefaultSettings())
	if err != nil {
		return false, err
	}
	if !prep.HasWork() {
		return false, nil
	}
	return true, engine.runCompaction(ctx, yield, false, rt.client)
}

// runCompaction prepares and appends one compaction entry, emitting UI events.
// Called from the overflow recovery path and from the tool-round boundary
// (model request via the context tool). The PrepareCompact here is deliberate
// re-validation, not waste: entries appended since requestCompact checked
// change what can be compacted, and a silent no-op (already compacted) is
// correct at the boundary, where an error would fail the turn.
func (engine *Engine) runCompaction(
	ctx context.Context,
	yield func(session.Event, error) bool,
	manual bool,
	client *llmclient.Client,
) error {
	settings := compaction.DefaultSettings()
	prepare := compaction.PrepareCompact
	if manual {
		prepare = compaction.PrepareCompactManual
	}
	prep, err := prepare(engine.sessionRef().PathEntries(), settings)
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return nil
	}

	id := fmt.Sprintf("compaction-%d", time.Now().UnixNano())
	if !yield(session.CompactionStarted{}, nil) {
		return errEventConsumerStopped
	}

	result, err := compaction.Compact(ctx, *prep, client)
	if err != nil {
		if !yield(session.CompactionComplete{ID: id, Failed: true}, nil) {
			return errEventConsumerStopped
		}
		return err
	}
	beforeTokens := result.TokensBefore
	if beforeTokens == 0 {
		beforeTokens = estimateContextTokens(engine.sessionRef().BuildContext())
	}
	compactedContext := make([]llm.Message, 0, len(prep.RecentMessages)+1)
	compactedContext = append(compactedContext, llm.Message{Role: llm.RoleUser, Content: result.Summary})
	compactedContext = append(compactedContext, prep.RecentMessages...)
	record := session.Compaction{
		Summary:            result.Summary,
		FirstKeptEntryID:   result.FirstKeptEntryID,
		TokensBefore:       beforeTokens,
		TokensAfter:        estimateContextTokens(compactedContext),
		MessagesSummarized: len(prep.MessagesToSummarize) + len(prep.TurnPrefixMessages),
		MessagesKept:       len(prep.RecentMessages),
		Details:            result.Details,
	}
	if err := engine.sessionRef().AppendCompaction(record); err != nil {
		if !yield(session.CompactionComplete{ID: id, Failed: true}, nil) {
			return errEventConsumerStopped
		}
		return err
	}
	// Fresh context: the next pressure crossing may advise again.
	engine.rearmCompactAdvice()
	if !yield(session.CompactionComplete{ID: id, Compaction: record}, nil) {
		return errEventConsumerStopped
	}
	return nil
}

// CompactNow runs a user-initiated compaction (/compact): summarize the
// history now, regardless of the auto-compaction threshold. yield receives
// the UI events (CompactionStarted/Complete); returning false cancels.
// An error means there was nothing to compact or the summary request
// failed — the caller surfaces it to the user.
func (engine *Engine) CompactNow(ctx context.Context, yield func(session.Event) bool) error {
	// Same guards as requestCompact: an immediate answer beats a background
	// round-trip for the "nothing to compact yet" case.
	prep, err := compaction.PrepareCompactManual(engine.sessionRef().PathEntries(), compaction.DefaultSettings())
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return errors.New("nothing to compact: no older turn is available to summarize yet")
	}
	err = engine.runCompaction(ctx, func(ev session.Event, _ error) bool {
		return yield(ev)
	}, true, engine.roundSnapshot().client)
	if errors.Is(err, errEventConsumerStopped) {
		return context.Canceled
	}
	return err
}

// requestCompact validates and records a model-requested compaction. The
// engine applies it at the next tool-round boundary (see Loop); the
// transcript itself stays append-only. The PrepareCompact here is an early
// answer to the model ("nothing to compact yet"), not a cached decision:
// runCompaction re-prepares at the boundary on fresh state.
func (engine *Engine) requestCompact() error {
	if engine.pendingCompact {
		return errors.New("compaction already scheduled for this round boundary")
	}
	prep, err := compaction.PrepareCompactManual(engine.sessionRef().PathEntries(), compaction.DefaultSettings())
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return errors.New("nothing to compact: no uncompacted history to summarize yet")
	}
	engine.pendingCompact = true
	return nil
}
