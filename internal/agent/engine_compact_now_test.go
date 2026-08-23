package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/session"
)

func TestEngineCompactNowNothingToCompact(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	err := engine.CompactNow(t.Context(), func(session.Event) bool { return true })
	require.ErrorContains(t, err, "nothing to compact")
}

func TestEngineCompactNowRunsCompaction(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	var events []session.Event
	err := engine.CompactNow(t.Context(), func(ev session.Event) bool {
		events = append(events, ev)
		return true
	})
	require.NoError(t, err)
	require.Len(t, events, 2, "Started then Complete")
	_, started := events[0].(session.CompactionStarted)
	require.True(t, started)
	complete, ok := events[1].(session.CompactionComplete)
	require.True(t, ok)
	require.False(t, complete.Failed)

	// No chat rounds; the seeded history splits into a history summary plus
	// a turn-prefix summary (same shape as the boundary-compaction test).
	require.Zero(t, streams.Load(), "compact must not start chat rounds")
	require.Len(t, bodies(), 2)

	var found bool
	for _, entry := range engine.session.PathEntries() {
		if _, ok := entry.(session.CompactionEntry); ok {
			found = true
		}
	}
	require.True(t, found, "compaction entry must land in the session")
}
