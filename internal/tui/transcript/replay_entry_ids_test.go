package transcript_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

// streamingTextServer answers one turn as three text deltas, so the assistant
// row is patched several times before the round ends.
func streamingTextServer(t *testing.T) *httptest.Server {
	t.Helper()
	chunk := func(text string) string {
		payload, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{"role": "assistant", "content": text},
			}},
		})
		require.NoError(t, err)
		return "data: " + string(payload) + "\n\n"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range []string{"one ", "two ", "three"} {
			_, _ = fmt.Fprint(w, chunk(part))
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestEngine(t *testing.T, baseURL string) *agent.Engine {
	t.Helper()
	engine, err := agent.NewEngine(agent.EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: baseURL, APIKey: "x"},
		SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
		Gate:        permission.AllowAll{},
	})
	require.NoError(t, err)
	return engine
}

func rowIDs(snap session.Snapshot) []string {
	ids := make([]string, 0, len(snap.Messages))
	for _, msg := range snap.Messages {
		ids = append(ids, msg.ID)
	}
	return ids
}

// TestLiveRowIDsMatchReplayedEntries is the regression for the defect the
// rewind, fork and aside anchors stand on: a live transcript row used to
// carry an id of its own while the session entry behind it got another, so
// the same message answered to two different names depending on whether the
// session had been reopened. The row a live turn draws and the row a replay
// of the very same file draws must be one id.
func TestLiveRowIDsMatchReplayedEntries(t *testing.T) {
	engine := newTestEngine(t, streamingTextServer(t).URL)

	// What the submitter does for a prompt that runs right away: it draws the
	// row itself, under the id the controller handed back, and the same id
	// rides the turn into the session.
	const rowID = "0f1e2d3c4b5a6978"
	live := session.Apply(session.Snapshot{}, session.UserAppend{ID: rowID, Text: "hello"})

	var streamed []string
	for event, err := range engine.Loop(t.Context(), "hello", agent.LoopOpts{UserID: rowID}) {
		require.NoError(t, err)
		if update, ok := event.(session.AssistantMessageUpdate); ok {
			streamed = append(streamed, update.Message.ID)
		}
		live = session.Apply(live, event)
	}

	replayed := transcript.ReplaySnapshot(engine.Session().PathEntries())
	assert.Equal(t, rowIDs(replayed), rowIDs(live),
		"a live turn and a replay of the file it wrote must name the same rows")

	require.Len(t, live.Messages, 2, "the turn is one prompt and one answer")
	assert.Equal(t, rowID, live.Messages[0].ID, "the prompt keeps the id its row was drawn under")

	require.NotEmpty(t, streamed, "the round streams before it ends")
	for _, id := range streamed {
		assert.Equal(t, live.Messages[1].ID, id,
			"the assistant id is stable from the first token, or the tail patch would draw a new row")
	}
}
