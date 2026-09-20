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
)

// seedTwoTurns records two finished turns and returns the entry ids of the
// second turn's prompt and answer.
func seedTwoTurns(t *testing.T, engine *Engine) (prompt, answer string) {
	t.Helper()
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "first question"},
		llm.Message{Role: llm.RoleAssistant, Content: "first answer"},
		llm.Message{Role: llm.RoleUser, Content: "second question"},
		llm.Message{Role: llm.RoleAssistant, Content: "second answer"},
	))
	items := engine.ContextReport().Items
	require.Len(t, items, 4)
	return items[2].EntryID, items[3].EntryID
}

// A rewind through the engine moves the cursor and drops the stale context
// cache with it, so the model's very next request is built on the new branch.
func TestEngineRewindDropsTheTailFromTheNextRequest(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	prompt, _ := seedTwoTurns(t, engine)
	require.Len(t, engine.session.BuildContext(), 4, "the cache is warm before the rewind")

	result, err := engine.Rewind(prompt)
	require.NoError(t, err)
	assert.Equal(t, "second question", result.Prompt)

	msgs := engine.session.BuildContext()
	require.Len(t, msgs, 2, "the second turn left the model's view")
	assert.Equal(t, "first answer", msgs[1].Content)
}

// Undo through the engine puts the cursor back and drops the cache again.
func TestEngineUndoRewindRestoresTheBranch(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	prompt, _ := seedTwoTurns(t, engine)
	_, err := engine.Rewind(prompt)
	require.NoError(t, err)

	_, err = engine.UndoRewind()
	require.NoError(t, err)
	require.Len(t, engine.session.BuildContext(), 4)
}

// The engine lists the places a cut is allowed: both prompts and the first
// answer of two finished turns. The second answer is where the cursor already
// stands, so cutting there would move nothing and it is not offered.
func TestEngineTurnBoundaries(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	prompt, answer := seedTwoTurns(t, engine)

	boundaries := engine.TurnBoundaries()
	require.Len(t, boundaries, 3)
	assert.Equal(t, prompt, boundaries[2].EntryID)
	for _, boundary := range boundaries {
		assert.NotEqual(t, answer, boundary.EntryID)
	}
	assert.NotContains(t, engine.RewindOffers(), answer)
	assert.Contains(t, engine.RewindOffers(), prompt)
}

// Moving the cursor mid-turn would hand the model a round it never ran, so
// the engine refuses while the loop is inside a request.
func TestEngineRefusesToMoveTheCursorDuringATurn(t *testing.T) {
	var (
		mu        sync.Mutex
		engine    *Engine
		rewindErr error
		undoErr   error
		anchor    string
		asked     bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		if !asked {
			asked = true
			_, rewindErr = engine.Rewind(anchor)
			_, undoErr = engine.UndoRewind()
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var err error
	engine, err = NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	anchor, _ = seedTwoTurns(t, engine)

	for range engine.Loop(t.Context(), "third question", LoopOpts{}) { //nolint:revive // the turn is what matters
	}

	mu.Lock()
	defer mu.Unlock()
	require.True(t, asked, "the handler ran, so the refusal was measured inside a turn")
	require.ErrorIs(t, rewindErr, ErrTurnRunning)
	require.ErrorIs(t, undoErr, ErrTurnRunning)
	assert.False(t, engine.TurnRunning(), "the turn released the guard on its way out")

	_, err = engine.Rewind(anchor)
	require.NoError(t, err, "the same rewind goes through once the turn is over")
}

// A turn recorded after a rewind grows a new branch: the model's context
// skips the one the cursor left, and the log keeps both.
func TestEngineTurnAfterRewindContinuesFromTheNewLeaf(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	prompt, abandoned := seedTwoTurns(t, engine)
	_, err := engine.Rewind(prompt)
	require.NoError(t, err)

	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "second question, reworded"},
		llm.Message{Role: llm.RoleAssistant, Content: "a different answer"},
	))

	msgs := engine.session.BuildContext()
	require.Len(t, msgs, 4)
	assert.Equal(t, "second question, reworded", msgs[2].Content)
	for _, msg := range msgs {
		assert.NotEqual(t, "second answer", msg.Content)
	}

	for _, entry := range engine.session.PathEntries() {
		assert.NotEqual(t, abandoned, entry.GetID())
	}
}

// A prompt sent with skills attached carries a harness paragraph into the
// log. What comes back to the composer is what the user wrote: sent again as
// it stands, the instruction would reach the model a second time.
func TestRewindHandsBackWhatTheUserTypedWithoutTheSkillParagraph(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	composed := engine.composeUserPrompt(nil, []string{"proofread"}, "check my prose", "check my prose")
	require.Contains(t, composed, skillReadInstruction, "the fixture is a prompt sent with a skill")

	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: composed},
		llm.Message{Role: llm.RoleAssistant, Content: "checked"},
	))
	anchor := engine.ContextReport().Items[0].EntryID

	result, err := engine.Rewind(anchor)
	require.NoError(t, err)
	assert.Equal(t, "check my prose", result.Prompt)
}

// The paragraph is built from the same constant the strip looks for, so a
// reworded instruction cannot silently start leaking into the composer.
func TestTheSkillParagraphAndTheStripAgree(t *testing.T) {
	instruction := pendingSkillsInstruction(t.TempDir(), []string{"proofread"})
	require.NotEmpty(t, instruction)
	assert.Empty(t, userTypedPrompt(instruction),
		"an instruction with no words of the user's leaves nothing behind")
	assert.Equal(t, "my words", userTypedPrompt(instruction+"\n\nmy words"))
	assert.Equal(t, "untouched", userTypedPrompt("untouched"))
}

// The line the picker offers and the text the composer receives are one
// answer, not two: both are what the user typed, without the harness
// paragraph the turn put in front of it.
func TestTheBoundaryPreviewMatchesWhatTheComposerGets(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	composed := engine.composeUserPrompt(nil, []string{"proofread"}, "check my prose", "check my prose")
	require.Contains(t, composed, skillReadInstruction)

	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: composed},
		llm.Message{Role: llm.RoleAssistant, Content: "checked"},
	))

	boundaries := engine.TurnBoundaries()
	// One, not two: the answer is where the cursor stands.
	require.Len(t, boundaries, 1)
	assert.Equal(t, "check my prose", boundaries[0].Prompt)
	assert.Equal(t, "before check my prose", boundaries[0].Preview)

	result, err := engine.Rewind(boundaries[0].EntryID)
	require.NoError(t, err)
	assert.Equal(t, boundaries[0].Prompt, result.Prompt,
		"the picker and the composer say the same thing")
}
