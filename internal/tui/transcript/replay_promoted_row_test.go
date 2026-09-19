package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// TestPromotedRowIDMatchesEntry covers the other delivery point: a prompt that
// waited in the queue has no row until the engine promotes one, and that row
// must name the entry the same turn appended. The row is owed here, which is
// what separates a queued prompt from a submitted one now that both carry an
// id.
func TestPromotedRowIDMatchesEntry(t *testing.T) {
	engine := newTestEngine(t, streamingTextServer(t).URL)

	const rowID = "9a8b7c6d5e4f3021"
	live := session.Snapshot{}
	for event, err := range engine.Loop(t.Context(), "queued", agent.LoopOpts{
		UserID:          rowID,
		UserRowOwed:     true,
		UserDisplayText: "queued",
	}) {
		require.NoError(t, err)
		live = session.Apply(live, event)
	}

	replayed := transcript.ReplaySnapshot(engine.Session().PathEntries())
	assert.Equal(t, rowIDs(replayed), rowIDs(live),
		"a promoted row and the replayed entry behind it must name the same message")
	require.NotEmpty(t, live.Messages)
	assert.Equal(t, rowID, live.Messages[0].ID, "the promoted row keeps the id it queued with")
}
