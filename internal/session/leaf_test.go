package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// turnIDs are the entry ids of one recorded turn.
type turnIDs struct {
	Prompt string
	Call   string
	Result string
	Answer string
}

// recordTurn writes a full turn: a prompt, an answer that calls a tool, the
// tool result, and the answer that closes the turn. Only the first and the
// last of those four are turn boundaries.
func recordTurn(t *testing.T, manager *session.Manager, prompt, answer string) turnIDs {
	t.Helper()
	var ids turnIDs
	var err error
	ids.Prompt, err = manager.Append(llm.Message{Role: llm.RoleUser, Content: prompt})
	require.NoError(t, err)
	ids.Call, err = manager.AppendAssistant(llm.Message{
		Role:      llm.RoleAssistant,
		Content:   "looking it up",
		ToolCalls: []llm.ToolCall{{ID: "call-" + prompt, Function: llm.Function{Name: "read"}}},
	}, "test-model", "")
	require.NoError(t, err)
	ids.Result, err = manager.Append(llm.Message{
		Role:       llm.RoleTool,
		Content:    "file contents",
		ToolCallID: "call-" + prompt,
	})
	require.NoError(t, err)
	ids.Answer, err = manager.AppendAssistant(llm.Message{
		Role:    llm.RoleAssistant,
		Content: answer,
	}, "test-model", "")
	require.NoError(t, err)
	return ids
}

func contextIDs(manager *session.Manager) []string {
	entries := manager.BuildContext()
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.GetID())
	}
	return ids
}

// A rewind to a prompt cuts the context before it: the prompt itself leaves
// the model's view, and its text comes back so the user can send it again.
func TestRewindToAPromptCutsBeforeItAndReturnsTheText(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	result, err := manager.Rewind(second.Prompt)
	require.NoError(t, err)
	assert.Equal(t, first.Answer, result.Target, "the cursor stands on what the prompt was appended after")
	assert.Equal(t, second.Answer, result.From)
	assert.Equal(t, "two", result.Prompt, "the prompt text goes back to the composer")

	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(manager),
		"everything from the cut prompt onwards left the context")
}

// A rewind to an answer cuts after it: the answer stays, because a turn that
// finished is a whole thing the model may keep.
func TestRewindToAnAnswerKeepsItInContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	recordTurn(t, manager, "two", "answer two")

	result, err := manager.Rewind(first.Answer)
	require.NoError(t, err)
	assert.Equal(t, first.Answer, result.Target)
	assert.Empty(t, result.Prompt, "an answer anchor hands nothing to the composer")
	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(manager))
}

// Cutting inside a turn would hand the model a tool call without its result,
// so the middle of a turn is refused by name.
func TestRewindRefusesInsideATurn(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")

	for _, id := range []string{turn.Call, turn.Result} {
		_, err := manager.Rewind(id)
		var notBoundary *session.NotTurnBoundaryError
		require.ErrorAs(t, err, &notBoundary, "the middle of a turn is not a place to cut at")
		assert.Contains(t, err.Error(), id, "the refusal names the entry it was asked about")
		assert.Contains(t, err.Error(), "turn boundary")
	}
	assert.Equal(t, turn.Answer, manager.LeafID(), "a refused rewind moves nothing")
}

// An entry that belongs to no branch of this session is refused the same way.
func TestRewindRefusesAnUnknownEntry(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")

	_, err := manager.Rewind("nothing-like-this")
	require.Error(t, err)
}

// Rewinding to the first prompt of a session empties the context: there is
// nothing before it, and the cursor says so rather than pretending.
func TestRewindToTheFirstPromptEmptiesTheContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")

	result, err := manager.Rewind(first.Prompt)
	require.NoError(t, err)
	assert.Empty(t, result.Target)
	assert.Empty(t, manager.LeafID())
	assert.Empty(t, manager.BuildContext())
}

// The cursor move is metadata: it is written to the log, but the model never
// sees it, exactly like the session title.
func TestACursorMoveNeverEntersTheContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	recordTurn(t, manager, "two", "answer two")

	_, err := manager.Rewind(first.Answer)
	require.NoError(t, err)
	for _, entry := range manager.BuildContext() {
		assert.NotEqual(t, session.EntryLeaf, entry.GetType())
	}
	assert.Equal(t, 10, manager.Len(),
		"the header, both turns and the move: nothing was taken out of the log")
}

// The turn after a rewind grows a new branch from the new cursor, and the
// branch the cursor left keeps every entry it had.
func TestATurnAfterARewindStartsANewBranch(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	abandoned := recordTurn(t, manager, "two", "answer two")

	_, err := manager.Rewind(abandoned.Prompt)
	require.NoError(t, err)
	third := recordTurn(t, manager, "three", "answer three")

	assert.Equal(t,
		[]string{
			first.Prompt, first.Call, first.Result, first.Answer,
			third.Prompt, third.Call, third.Result, third.Answer,
		},
		contextIDs(manager),
		"the new branch skips the one the cursor left")

	branch := manager.GetBranch(abandoned.Answer)
	require.NotEmpty(t, branch, "the abandoned branch is still in the log")
	assert.Equal(t, abandoned.Answer, branch[0].GetID())
}

// Undo puts the cursor back on the branch the rewind left, even when a whole
// turn has been recorded on the new one since.
func TestUndoRewindReturnsToTheBranchLeftBehind(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")
	abandoned := recordTurn(t, manager, "two", "answer two")

	_, err := manager.Rewind(abandoned.Prompt)
	require.NoError(t, err)
	recordTurn(t, manager, "three", "answer three")

	result, err := manager.UndoRewind()
	require.NoError(t, err)
	assert.Equal(t, abandoned.Answer, result.Target)
	assert.Equal(t, abandoned.Answer, manager.LeafID())
	assert.Contains(t, contextIDs(manager), abandoned.Prompt)
}

// Undo is itself a move, so undoing twice lands where the pair started. It is
// the price of keeping one rule instead of two, and the file says as much.
func TestTwoUndosLandWhereTheyStarted(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	_, err := manager.Rewind(first.Answer)
	require.NoError(t, err)
	require.Equal(t, first.Answer, manager.LeafID())

	_, err = manager.UndoRewind()
	require.NoError(t, err)
	require.Equal(t, second.Answer, manager.LeafID())

	_, err = manager.UndoRewind()
	require.NoError(t, err)
	assert.Equal(t, first.Answer, manager.LeafID(), "the second undo goes back to the rewind")
}

// Undo with nothing behind it says so instead of guessing.
func TestUndoRewindWithoutAMoveRefuses(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")

	_, err := manager.UndoRewind()
	require.ErrorIs(t, err, session.ErrNothingToUndo)
}

// Cutting where the cursor already stands would write a move that moves
// nothing, so it is refused and the log stays clean.
func TestRewindToTheCurrentCursorRefuses(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")
	before := manager.Len()

	_, err := manager.Rewind(turn.Answer)
	require.ErrorIs(t, err, session.ErrCursorAlreadyThere)
	assert.Equal(t, before, manager.Len())
}

// Reopening the file restores the cursor the rewind left, so a session
// resumed after a rewind carries on where the user put it, not where the
// abandoned branch ends.
func TestReloadRestoresTheCursorAfterARewind(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)

	first := recordTurn(t, manager, "one", "answer one")
	abandoned := recordTurn(t, manager, "two", "answer two")
	_, err = manager.Rewind(abandoned.Prompt)
	require.NoError(t, err)
	before := contextIDs(manager)
	path := manager.File()
	require.NoError(t, manager.Close())

	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	assert.Equal(t, first.Answer, reopened.LeafID())
	assert.Equal(t, before, contextIDs(reopened))

	raw, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	assert.Contains(t, string(raw), abandoned.Answer,
		"the abandoned branch is still in the file: the log is append-only")
	assert.Equal(t, 1, strings.Count(string(raw), `"type":"leaf"`))
}

// A rewind that empties the context survives the reload too: an empty cursor
// is a state of its own, not a missing one.
func TestReloadRestoresAnEmptyCursor(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)

	first := recordTurn(t, manager, "one", "answer one")
	_, err = manager.Rewind(first.Prompt)
	require.NoError(t, err)
	path := manager.File()
	require.NoError(t, manager.Close())

	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	assert.Empty(t, reopened.LeafID())
	assert.Empty(t, reopened.BuildContext())
}

// A move the file never received must not move the cursor in memory either.
func TestAFailedFlushLeavesTheCursorAlone(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")
	require.NoError(t, os.Remove(manager.File()))
	require.NoError(t, os.Mkdir(manager.File(), 0o755))

	_, err = manager.Rewind(second.Prompt)
	require.Error(t, err)
	assert.Equal(t, second.Answer, manager.LeafID())
	assert.Contains(t, contextIDs(manager), second.Prompt)
}

// A move pointing at an entry the file does not hold is a broken log. Taking
// it would leave the cursor dangling, and the context builder would fall back
// to the last line of the file and land somewhere right only by luck.
func TestLoadRefusesACursorMoveWithAnUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	recordTurn(t, manager, "one", "answer one")
	path := manager.File()
	require.NoError(t, manager.Close())

	line := `{"type":"leaf","id":"move-1","parentID":null,` +
		`"timestamp":"2026-09-16T09:00:00Z","target":"no-such-entry","from":""}` + "\n"
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString(line)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	_, err = session.OpenSession(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-entry")
}

// An unterminated reminder block is the transcript's rule, not a special case
// of ours: the feed shows the text as the user's, so a rewind offers it as a
// boundary. Two spellings of "what the user typed" would disagree about which
// rows carry buttons at all.
func TestAnUnterminatedReminderReadsAsTheUsersText(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	prompt, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "<system-reminder>never closed, so this is prose",
	})
	require.NoError(t, err)
	_, err = manager.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "ok"}, "m", "")
	require.NoError(t, err)

	boundary, err := session.TurnBoundaryAt(manager.BuildContext(), prompt)
	require.NoError(t, err)
	assert.Equal(t, "<system-reminder>never closed, so this is prose", boundary.Prompt)
}

// From is checked at load with Target, because an undo moves onto it. An
// unchecked one is the same dangling cursor, held back until /rewind back.
func TestLoadRefusesACursorMoveWithAnUnknownOrigin(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	turn := recordTurn(t, manager, "one", "answer one")
	path := manager.File()
	require.NoError(t, manager.Close())

	line := `{"type":"leaf","id":"move-1","parentID":null,` +
		`"timestamp":"2026-09-16T09:00:00Z","target":"` + turn.Prompt + `","from":"gone"}` + "\n"
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString(line)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	_, err = session.OpenSession(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gone")
}

// A cut whose target is the cursor moves nothing, and the three shapes that
// reach it are the ones a reader meets first: the answer a finished turn left
// the cursor on, a prompt whose turn never produced an answer, and whatever
// row the previous rewind landed on.
func TestACutAtTheCursorIsNotOffered(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	t.Run("the answer a finished turn ended on", func(t *testing.T) {
		require.Equal(t, second.Answer, manager.LeafID())
		assert.False(t, manager.RewindMovesCursor(second.Answer))
		assert.True(t, manager.RewindMovesCursor(second.Prompt))
		for _, boundary := range manager.TurnBoundaries() {
			assert.NotEqual(t, second.Answer, boundary.EntryID)
		}
	})

	t.Run("a prompt whose turn never answered", func(t *testing.T) {
		orphan, err := manager.Append(llm.Message{Role: llm.RoleUser, Content: "three"})
		require.NoError(t, err)
		// The prompt itself became the end of the context, so cutting before
		// it does move the cursor: back onto the answer it was appended to.
		// This row keeps its button, and taking it up works.
		require.Equal(t, orphan, manager.LeafID())
		assert.True(t, manager.RewindMovesCursor(orphan))
		// Cutting after the answer the prompt hangs off leads to the same
		// place by the other route, and it moves the cursor too.
		assert.True(t, manager.RewindMovesCursor(second.Answer))
	})

	t.Run("a row the session never recorded", func(t *testing.T) {
		// A prompt drawn into the feed for a turn that failed before the log
		// received it. There is nothing to cut back to, and the row must not
		// say otherwise.
		assert.False(t, manager.RewindMovesCursor("never-recorded"))
	})

	t.Run("the row the previous rewind landed on", func(t *testing.T) {
		_, err := manager.Rewind(second.Prompt)
		require.NoError(t, err)
		require.Equal(t, first.Answer, manager.LeafID())
		assert.False(t, manager.RewindMovesCursor(first.Answer),
			"the cursor stands here now, so cutting here again moves nothing")
		assert.True(t, manager.RewindMovesCursor(first.Prompt))
		for _, boundary := range manager.TurnBoundaries() {
			assert.NotEqual(t, first.Answer, boundary.EntryID)
		}
	})
}

// A cut at the cursor is still refused by name if something asks for it
// anyway: the offer is the first guard, not the only one.
func TestACutAtTheCursorIsStillRefusedWhenAsked(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")

	_, err := manager.Rewind(turn.Answer)
	require.ErrorIs(t, err, session.ErrCursorAlreadyThere)
}
