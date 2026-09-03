package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// A turn that never ends must escalate the compaction ladder per tool round,
// not per turn: the runaway loop from session 55cf07d2 ran 101 rounds inside
// one turn because pressure was only noted at turn end. Four over-threshold
// rounds bring the ladder to its stop; the stop buys the model exactly one
// offer round (reminder-format directive, context tool only) before the
// engine halts and waits for a manual compaction.
func TestLoopMidTurnPressureStopsRunawayAndOffersCompaction(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(n int32) string {
		switch n {
		case 1, 2, 3, 4:
			return sseToolCallChunk(fmt.Sprintf("call_%d", n), "context", `{}`)
		default:
			return sseTextChunk()
		}
	})

	engine := newContextTestEngine(t, server.URL, 30000)
	seedTwoTurnHistory(t, engine) // 25000 provider tokens over the 13616 threshold

	var lastErr error
	for _, err := range engine.Loop(t.Context(), "keep working", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	require.ErrorIs(t, lastErr, ErrCompactionRequired, "the runaway turn ends in the compaction-required stop")
	require.True(t, engine.compactStopActive(), "without a compaction the engine stops")
	require.Equal(t, int32(5), streams.Load(), "four pressure rounds plus exactly one offer round")

	snapshot := bodies()
	require.Len(t, snapshot, 5, "no compaction ran: every request is a streaming round")
	require.Contains(t, snapshot[4], "system-reminder", "the offer round is driven by a reminder-format directive")
	require.Contains(t, snapshot[4], "final round", "the directive says this is the last chance to compact")
	require.Contains(t, snapshot[4], `action\":\"compact\"`, "the directive names the compact call")

	// The stop holds at the door: a new turn is refused, nothing is sent.
	var nextErr error
	for _, err := range engine.Loop(t.Context(), "again", LoopOpts{}) {
		if err != nil {
			nextErr = err
		}
	}
	require.ErrorIs(t, nextErr, ErrCompactionRequired, "a stopped engine refuses new turns")
	require.Equal(t, int32(5), streams.Load(), "no inference request leaves a stopped engine")
}

// The offer round accepted: the model summarizes and calls the context tool
// with {"action":"compact"}, the compaction lands at the round boundary, the
// ladder rearms and the SAME turn continues on the compacted context instead
// of dying with an error.
func TestLoopMidTurnOfferCompactionRearmsAndTurnContinues(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(n int32) string {
		switch n {
		case 1, 2, 3, 4:
			return sseToolCallChunk(fmt.Sprintf("call_%d", n), "context", `{}`)
		case 5:
			return sseToolCallChunk("call_5", "context", `{"action":"compact"}`)
		default:
			return sseTextChunk()
		}
	})

	engine := newContextTestEngine(t, server.URL, 30000)
	seedTwoTurnHistory(t, engine)

	var (
		lastErr   error
		compacted bool
	)
	for ev, err := range engine.Loop(t.Context(), "keep working", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		if _, ok := ev.(session.CompactionComplete); ok {
			compacted = true
		}
	}
	require.NoError(t, lastErr, "a landed compaction in the offer round continues the turn")
	require.True(t, compacted, "the offer round's compact call runs a compaction")
	require.False(t, engine.compactStopActive(), "the compaction resets the stop")
	// The ladder restarted from the bottom: the fake provider still reports
	// 26000 tokens over this test's tiny threshold, so the turn-end note strikes
	// once more — a fresh soft strike, not the remains of the stop ladder.
	require.Equal(t, 1, engine.compactStrikes, "the ladder rearmed and started over")
	require.False(t, engine.compactHardMode())
	require.Equal(t, int32(6), streams.Load(), "four rounds, the offer round, one round after the compaction")

	snapshot := bodies()
	require.Len(t, snapshot, 7, "six streaming rounds plus one summary request")
	require.Contains(t, snapshot[4], "final round", "round five runs under the offer directive")
	// snapshot[5] is the summary request itself; the turn's next round runs on
	// the compacted context — the summary sentinel is what the model sees.
	require.Contains(t, snapshot[5], "SEED-A-SENTINEL", "the summary request carries the old history")
	require.Contains(t, snapshot[6], "SUMMARY-OF-OLD-HISTORY", "the turn continues on the compacted context")
}

// The hard guarantee: an inference whose estimated context exceeds the model
// window is never sent at all — the runaway from session 55cf07d2 died when a
// ~211k estimate hit a 200k window. Here the prompt alone pushes the estimate
// over a tiny window; the provider must not see a single request.
func TestLoopRefusesInferenceOverContextWindow(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused summary", func(int32) string {
		return sseTextChunk()
	})

	engine := newContextTestEngine(t, server.URL, 1000)

	prompt := strings.Repeat("x", 16*1024) // ~4096 estimated tokens over a 1000-token window
	var lastErr error
	for _, err := range engine.Loop(t.Context(), prompt, LoopOpts{}) {
		if err != nil {
			lastErr = err
		}
	}
	require.ErrorIs(t, lastErr, ErrCompactionRequired, "an over-window inference is refused before the request")
	require.Zero(t, streams.Load(), "no inference request may leave the engine")
	require.Empty(t, bodies())

	// The refusal repeats: the engine waits for a compaction, not a retry.
	var nextErr error
	for _, err := range engine.Loop(t.Context(), "again", LoopOpts{}) {
		if err != nil {
			nextErr = err
		}
	}
	require.ErrorIs(t, nextErr, ErrCompactionRequired)
	require.Zero(t, streams.Load(), "still nothing sent")
}
