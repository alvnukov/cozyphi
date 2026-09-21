package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// persistedSession is a session written to disk, which is the only kind that
// can be forked: the copy is a second file beside the first one.
func persistedSession(t *testing.T) *session.Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir),
		session.WithShouldFlush(true),
		session.WithModel("test-model"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

// forkHeader reads the first line of a session file back as its header.
func forkHeader(t *testing.T, path string) session.SessionHeader {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	first, _, found := strings.Cut(string(data), "\n")
	require.True(t, found, "the file has no first line")
	var header session.SessionHeader
	require.NoError(t, json.Unmarshal([]byte(first), &header))
	return header
}

// The whole point of a fork: a second session file carrying the branch up to
// the anchor, entry for entry and id for id, while the session it was taken
// from is left exactly as it was.
func TestForkCopiesTheBranchAndLeavesTheSourceAlone(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	before, err := os.ReadFile(source.File())
	require.NoError(t, err)

	forked, err := session.Fork(source, first.Answer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(forked),
		"the copy holds the branch up to the anchor, under the ids it had")
	assert.Equal(t, first.Answer, forked.LeafID(), "the copy starts where the anchor left off")
	assert.NotEqual(t, source.ID(), forked.ID())
	assert.Equal(t, filepath.Dir(source.File()), filepath.Dir(forked.File()),
		"the fork lands beside the session it came from")

	header := forkHeader(t, forked.File())
	assert.Equal(t, source.ID(), header.ParentSession, "the header names the session it came from")
	assert.Equal(t, first.Answer, header.ForkedFrom, "and the entry it stops at")
	assert.Equal(t, source.Cwd(), header.Cwd)

	after, err := os.ReadFile(source.File())
	require.NoError(t, err)
	assert.Equal(t, before, after, "the session that was forked is not written to")
	assert.Equal(t, second.Answer, source.LeafID(), "and its cursor has not moved")
}

// A turn recorded in the fork stays in the fork. The two files are separate
// conversations from the moment the copy is written.
func TestATurnInTheForkNeverReachesTheSource(t *testing.T) {
	source := persistedSession(t)
	turn := recordTurn(t, source, "one", "answer one")

	forked, err := session.Fork(source, turn.Answer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	before, err := os.ReadFile(source.File())
	require.NoError(t, err)

	recordTurn(t, forked, "only in the fork", "answer in the fork")

	after, err := os.ReadFile(source.File())
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.NotContains(t, string(after), "only in the fork")

	data, err := os.ReadFile(forked.File())
	require.NoError(t, err)
	assert.Contains(t, string(data), "only in the fork")
}

// Reopening the forked file gives back the conversation the copy was made
// of: the same context in the same order, ready for the next turn.
func TestTheForkedFileReopensWithTheCopiedBranch(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	recordTurn(t, source, "two", "answer two")

	forked, err := session.Fork(source, first.Answer)
	require.NoError(t, err)
	path := forked.File()
	require.NoError(t, forked.Close())

	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(reopened))
	assert.Equal(t, first.Answer, reopened.LeafID())
	assert.Equal(t, "test-model", reopened.Model())
}

// A fork cut at a prompt stops before it, exactly like a rewind: the prompt
// is not in the copy, and its text is what the boundary hands over.
func TestForkAtAPromptStopsBeforeIt(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	boundary, err := source.ForkBoundary(second.Prompt)
	require.NoError(t, err)
	assert.Equal(t, "two", boundary.Prompt, "the text of the prompt the copy stops before")

	forked, err := session.Fork(source, second.Prompt)
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(forked))
	assert.Equal(t, second.Prompt, forkHeader(t, forked.File()).ForkedFrom)
}

// A fork with no anchor copies the conversation as it stands, which is what
// /fork without an id asks for.
func TestForkWithoutAnAnchorCopiesTheWholeConversation(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	forked, err := session.Fork(source, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	assert.Equal(t, contextIDs(source), contextIDs(forked))
	assert.Equal(t, second.Answer, forkHeader(t, forked.File()).ForkedFrom,
		"the anchor recorded is the entry the cursor stood on")
	assert.NotEmpty(t, first.Answer)
}

// A fork taken after a rewind copies the branch the cursor stands on, not the
// one it left.
func TestForkFollowsTheCursorAfterARewind(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	_, err := source.Rewind(second.Prompt)
	require.NoError(t, err)

	forked, err := session.Fork(source, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(forked),
		"the branch the rewind left is not in the copy")
	assert.NotContains(t, contextIDs(forked), second.Answer)
}

// The context of the copy is the context of the source, compaction and all:
// the copy carries the compaction entry rather than the history it hides.
func TestForkKeepsTheCompactedShapeOfTheContext(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	_, err := source.AppendCompaction(session.Compaction{
		Summary:          "the first turn, summarized",
		FirstKeptEntryID: second.Prompt,
	})
	require.NoError(t, err)
	third := recordTurn(t, source, "three", "answer three")

	forked, err := session.Fork(source, third.Answer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })

	assert.Equal(t, contextIDs(source), contextIDs(forked),
		"the copy is shown exactly what the source is shown")
	assert.NotContains(t, contextIDs(forked), first.Prompt,
		"what the compaction hid is hidden in the copy too")

	data, err := os.ReadFile(forked.File())
	require.NoError(t, err)
	assert.Contains(t, string(data), "the first turn, summarized",
		"and the summary that stands for it is in the file")
}

// Anywhere inside a turn is refused by name, the same way a rewind is: the
// copy would hand the model a tool call without its result.
func TestForkInsideATurnIsRefused(t *testing.T) {
	source := persistedSession(t)
	turn := recordTurn(t, source, "one", "answer one")

	for _, anchor := range []string{turn.Call, turn.Result, "no-such-entry"} {
		_, err := session.Fork(source, anchor)
		require.Error(t, err, "anchor %q", anchor)
		var notBoundary *session.NotTurnBoundaryError
		require.ErrorAs(t, err, &notBoundary)
		assert.Equal(t, anchor, notBoundary.EntryID)
	}
}

// Two sessions cannot be forked: an empty one, because the copy would be
// empty, and one that was never written to disk, because a fork is a file.
func TestForkRefusesAnEmptyOrUnwrittenSession(t *testing.T) {
	source := persistedSession(t)
	_, err := session.Fork(source, "")
	require.ErrorIs(t, err, session.ErrNothingToFork)

	memory := session.NewManager(t.TempDir())
	recordTurn(t, memory, "one", "answer one")
	_, err = session.Fork(memory, "")
	require.ErrorIs(t, err, session.ErrForkNotPersisted)
}

// A closed session is read by nothing, a fork included.
func TestForkOfAClosedSessionIsRefused(t *testing.T) {
	source := persistedSession(t)
	recordTurn(t, source, "one", "answer one")
	require.NoError(t, source.Close())

	_, err := session.Fork(source, "")
	require.ErrorIs(t, err, os.ErrClosed)
}

// The places a fork may be taken include the one the cursor already stands
// on, which the rewind list leaves out: a copy of the whole conversation is
// the commonest fork there is.
func TestForkBoundariesKeepTheEntryTheCursorStandsOn(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	second := recordTurn(t, source, "two", "answer two")

	ids := make([]string, 0, 4)
	for _, boundary := range source.ForkBoundaries() {
		ids = append(ids, boundary.EntryID)
	}
	assert.Equal(t, []string{first.Prompt, first.Answer, second.Prompt, second.Answer}, ids)

	rewindIDs := make([]string, 0, 3)
	for _, boundary := range source.TurnBoundaries() {
		rewindIDs = append(rewindIDs, boundary.EntryID)
	}
	assert.NotContains(t, rewindIDs, second.Answer)
}

// A log written before forks existed has no parent and no anchor in its
// header, and it still opens.
func TestASessionFileWithoutForkFieldsStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-09-20T10-00-00_deadbeef.jsonl")
	lines := []string{
		`{"type":"EntrySession","id":"deadbeef","timestamp":"2026-09-20T10-00-00","cwd":"` + dir + `"}`,
		`{"type":"EntryMessage","id":"m1","parentID":null,"timestamp":"2026-09-20T10:00:01Z",` +
			`"message":{"role":"user","content":"one"}}`,
		`{"type":"EntryMessage","id":"m2","parentID":"m1","timestamp":"2026-09-20T10:00:02Z",` +
			`"message":{"role":"assistant","content":"answer one"}}`,
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))

	manager, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	assert.Equal(t, []string{"m1", "m2"}, contextIDs(manager))
	header := forkHeader(t, path)
	assert.Empty(t, header.ParentSession)
	assert.Empty(t, header.ForkedFrom)

	forked, err := session.Fork(manager, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })
	assert.Equal(t, "deadbeef", forkHeader(t, forked.File()).ParentSession,
		"an old log is still something to fork from")
}

// The prompt of a message sent with harness scaffolding is the user's own
// words: the boundary strips what the transcript strips.
func TestForkBoundaryReportsTheAnchorTheCursorStandsOn(t *testing.T) {
	source := persistedSession(t)
	turn := recordTurn(t, source, "one", "answer one")

	boundary, err := source.ForkBoundary("")
	require.NoError(t, err)
	assert.Equal(t, turn.Answer, boundary.EntryID)
	assert.Empty(t, boundary.Prompt, "a copy that stops after an answer hands nothing over")

	_, err = source.ForkBoundary(turn.Call)
	require.Error(t, err)

	_, err = source.Append(llm.Message{Role: llm.RoleUser, Content: "next"})
	require.NoError(t, err)
	boundary, err = source.ForkBoundary("")
	require.NoError(t, err)
	assert.Equal(t, "next", boundary.Prompt)
}

// A fork at the very first prompt copies nothing: the anchor is cut before,
// and there is nothing in front of it. The copy is an empty session with the
// prompt waiting to be asked, and it opens like any other.
func TestForkAtTheFirstPromptWritesAnEmptySession(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")

	forked, err := session.Fork(source, first.Prompt)
	require.NoError(t, err)
	path := forked.File()
	require.NoError(t, forked.Close())

	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	assert.Empty(t, reopened.BuildContext(), "the copy stops before the only prompt there was")
	assert.Empty(t, reopened.LeafID())
	assert.Equal(t, first.Prompt, forkHeader(t, path).ForkedFrom)

	id, err := reopened.Append(llm.Message{Role: llm.RoleUser, Content: "one, reworded"})
	require.NoError(t, err)
	assert.NotEmpty(t, id, "an empty copy is a session that takes turns like any other")
}

// A stream cut in half leaves an assistant message with a tool call and no
// result behind it, and the cursor stands on it. A fork nobody aimed anywhere
// copies up to the last whole turn instead of refusing with an id the user
// never typed.
func TestForkWithoutAnAnchorStepsBackOutOfATornTurn(t *testing.T) {
	source := persistedSession(t)
	first := recordTurn(t, source, "one", "answer one")
	_, err := source.Append(llm.Message{Role: llm.RoleUser, Content: "two"})
	require.NoError(t, err)
	_, err = source.AppendAssistant(llm.Message{
		Role:      llm.RoleAssistant,
		Content:   "looking it up",
		ToolCalls: []llm.ToolCall{{ID: "call-torn", Function: llm.Function{Name: "read"}}},
	}, "test-model", "")
	require.NoError(t, err)

	boundary, err := source.ForkBoundary("")
	require.NoError(t, err)
	assert.Equal(t, "two", boundary.Prompt,
		"the newest boundary is the prompt the torn turn started with")

	forked, err := session.Fork(source, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = forked.Close() })
	assert.Equal(t,
		[]string{first.Prompt, first.Call, first.Result, first.Answer},
		contextIDs(forked),
		"the copy holds the turn that finished and none of the one that did not")
}

// The refusal for a session with nothing to copy yet names no entry: there is
// no row the user clicked to name back.
func TestForkWithoutAnAnchorRefusesWithoutNamingAnEntry(t *testing.T) {
	source := persistedSession(t)
	_, err := source.Append(llm.Message{
		Role:       llm.RoleUser,
		Content:    "a tool result is nothing anybody typed",
		ToolCallID: "call-1",
	})
	require.NoError(t, err)

	_, err = session.Fork(source, "")
	require.ErrorIs(t, err, session.ErrNothingToFork)
	assert.NotContains(t, err.Error(), source.LeafID(), "the refusal names no entry")

	_, err = source.ForkBoundary("")
	require.ErrorIs(t, err, session.ErrNothingToFork)
}

// A named anchor is answered as named. The user chose that row, so a refusal
// that names it back is about the row they clicked.
func TestForkAtANamedEntryInsideATurnStillNamesIt(t *testing.T) {
	source := persistedSession(t)
	turn := recordTurn(t, source, "one", "answer one")

	_, err := session.Fork(source, turn.Call)
	require.Error(t, err)
	var notBoundary *session.NotTurnBoundaryError
	require.ErrorAs(t, err, &notBoundary)
	assert.Equal(t, turn.Call, notBoundary.EntryID)
}
