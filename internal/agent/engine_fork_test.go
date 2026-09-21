package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// newForkTestEngine builds an engine whose session is written to disk, which
// is what a fork needs: the copy is a second file beside the first one.
func newForkTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:1", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
			Model:      "fake",
		},
	})
	require.NoError(t, err)
	return engine
}

// The file a fork writes is nobody's the moment it is written: whatever shows
// the copy opens it the way it opens any other session, and would meet ErrBusy
// if this side were still holding it.
func TestEngineForkLeavesTheNewSessionForSomebodyElseToOpen(t *testing.T) {
	engine := newForkTestEngine(t)
	prompt, answer := seedTwoTurns(t, engine)

	result, err := engine.Fork(answer)
	require.NoError(t, err)
	assert.Equal(t, answer, result.Anchor)
	assert.NotEmpty(t, result.SessionID)
	assert.NotEqual(t, prompt, result.SessionID)

	opened, err := session.OpenSession(result.File)
	require.NoError(t, err, "the fork holds no session open")
	t.Cleanup(func() { _ = opened.Close() })

	entries := opened.BuildContext()
	require.Len(t, entries, 4, "the copy is the whole branch up to the anchor")
	assert.Equal(t, answer, opened.LeafID())
}

// A prompt sent with skills attached carries a harness paragraph into the
// log. What the composer of the new tab receives is what the user wrote.
func TestEngineForkHandsOverThePromptWithoutTheHarnessParagraph(t *testing.T) {
	engine := newForkTestEngine(t)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "first question"},
		llm.Message{Role: llm.RoleAssistant, Content: "first answer"},
		llm.Message{
			Role:    llm.RoleUser,
			Content: skillReadInstruction + "\nskills/one.md\n\nwhat do the skills say?",
		},
		llm.Message{Role: llm.RoleAssistant, Content: "second answer"},
	))
	items := engine.ContextReport().Items
	require.Len(t, items, 4)

	result, err := engine.Fork(items[2].EntryID)
	require.NoError(t, err)
	assert.Equal(t, "what do the skills say?", result.Prompt)
}

// Copying mid-turn would take the branch as the model is still writing it,
// so the engine refuses while the loop is inside a request.
func TestEngineRefusesToForkDuringATurn(t *testing.T) {
	var (
		mu      sync.Mutex
		engine  *Engine
		forkErr error
		anchor  string
		asked   bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		if !asked {
			asked = true
			_, forkErr = engine.Fork(anchor)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	var err error
	engine, err = NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
	})
	require.NoError(t, err)
	anchor, _ = seedTwoTurns(t, engine)

	for range engine.Loop(t.Context(), "third question", LoopOpts{}) { //nolint:revive // the turn is what matters
	}

	mu.Lock()
	defer mu.Unlock()
	require.True(t, asked, "the handler ran, so the refusal was measured inside a turn")
	require.ErrorIs(t, forkErr, ErrForkWhileTurnRunning)

	_, err = engine.Fork(anchor)
	require.NoError(t, err, "the same fork goes through once the turn is over")
}

// The engine lists every place a fork may be taken, the entry the cursor
// stands on included; the rewind list is the one that leaves it out.
func TestEngineForkBoundariesKeepTheCursorsEntry(t *testing.T) {
	engine := newForkTestEngine(t)
	_, answer := seedTwoTurns(t, engine)

	boundaries := engine.ForkBoundaries()
	require.Len(t, boundaries, 4)
	assert.Equal(t, answer, boundaries[3].EntryID)
	assert.Len(t, engine.TurnBoundaries(), 3)
}
