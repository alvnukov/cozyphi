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

func askAside(t *testing.T, manager *session.Manager, anchor, question, answer string) session.AsideEntry {
	t.Helper()
	scope, err := manager.AsideScope(anchor)
	require.NoError(t, err)
	entry := session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: session.NewEntryID()},
		Leaf:             scope.Leaf,
		Anchor:           scope.Anchor,
		Question:         question,
		Answer:           answer,
		Model:            "test-model",
		Usage:            llm.Usage{PromptTokens: 120, CompletionTokens: 8, TotalTokens: 128},
	}
	require.NoError(t, manager.AppendAside(entry))
	return entry
}

// The whole point of an aside: it is written down, it comes back on reload,
// and the model never sees it, neither in this process nor in the next one.
func TestAnAsideIsKeptButNeverEntersTheContext(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)

	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")
	before := contextIDs(manager)

	asked := askAside(t, manager, first.Answer, "what did we settle on?", "the first answer")

	assert.Equal(t, before, contextIDs(manager), "the aside is not in the context")
	assert.Equal(t, second.Answer, manager.LeafID(), "the cursor did not move")
	path := manager.File()
	require.NoError(t, manager.Close())

	raw, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), `"type":"aside"`))

	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	assert.Equal(t, before, contextIDs(reopened), "the aside stays out of the context after a reload")
	assert.Equal(t, second.Answer, reopened.LeafID(), "a reload does not follow the aside either")
	asides := reopened.Asides()
	require.Len(t, asides, 1)
	got := asides[0]
	assert.Equal(t, asked.ID, got.ID)
	assert.Equal(t, first.Answer, got.Anchor)
	assert.Equal(t, second.Answer, got.Leaf)
	assert.Equal(t, "what did we settle on?", got.Question)
	assert.Equal(t, "the first answer", got.Answer)
	assert.Equal(t, "test-model", got.Model)
	assert.Equal(t, 128, got.Usage.TotalTokens)
	assert.Nil(t, got.GetParent(), "an aside is no node of the conversation")
}

// The next turn after an aside grows from the leaf the aside was asked at,
// not from the aside.
func TestATurnAfterAnAsideGrowsFromTheSameLeaf(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")
	askAside(t, manager, "", "side?", "yes")

	next, err := manager.Append(llm.Message{Role: llm.RoleUser, Content: "two"})
	require.NoError(t, err)
	branch := manager.GetBranch(next)
	require.GreaterOrEqual(t, len(branch), 2)
	assert.Equal(t, turn.Answer, branch[1].GetID(), "the prompt hangs off the answer, not off the aside")
}

// An empty anchor is the whole context and resolves to its last entry, so
// the record names a real place however the question was asked.
func TestAsideScopeWithoutAnAnchorIsTheWholeContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	scope, err := manager.AsideScope("")
	require.NoError(t, err)
	assert.Equal(t, second.Answer, scope.Anchor)
	assert.Equal(t, second.Answer, scope.Leaf)
	assert.Equal(t, contextIDs(manager), entryIDs(scope.Entries))
}

// A named anchor may be any message of the context, the middle of a turn
// included, and the context ends there.
func TestAsideScopeCutsTheContextAfterTheAnchor(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	scope, err := manager.AsideScope(first.Call)
	require.NoError(t, err)
	assert.Equal(t, first.Call, scope.Anchor)
	assert.Equal(t, second.Answer, scope.Leaf, "the leaf is where the cursor stands, not the anchor")
	assert.Equal(t, []string{first.Prompt, first.Call}, entryIDs(scope.Entries))
}

// What a side question may be asked about is what the feed shows: an entry
// the cursor left behind, a compaction and an unknown id are all refused, and
// so is a question in a session with nothing in it.
func TestAsideScopeRefusesWhatTheFeedDoesNotShow(t *testing.T) {
	empty := session.NewManager(t.TempDir())
	_, err := empty.AsideScope("")
	require.ErrorIs(t, err, session.ErrNothingToAsk)

	manager := session.NewManager(t.TempDir())
	recordTurn(t, manager, "one", "answer one")
	abandoned := recordTurn(t, manager, "two", "answer two")
	_, err = manager.Rewind(abandoned.Prompt)
	require.NoError(t, err)
	compactionID, err := manager.AppendCompaction(session.Compaction{Summary: "so far"})
	require.NoError(t, err)

	for _, id := range []string{abandoned.Answer, compactionID, "nothing-like-this"} {
		_, err := manager.AsideScope(id)
		var notAnchor *session.NotAsideAnchorError
		require.ErrorAs(t, err, &notAnchor, id)
		assert.Contains(t, err.Error(), id, "the refusal names the entry it was asked about")
	}
}

// Asides are invisible to everything that reads the path: rewind and fork
// offers, the anchors a new aside may take, and the copy a fork writes.
func TestAnAsideStaysOutOfBoundariesAnchorsAndForks(t *testing.T) {
	manager := persistedSession(t)
	recordTurn(t, manager, "one", "answer one")
	recordTurn(t, manager, "two", "answer two")
	turnsBefore := manager.TurnBoundaries()
	forksBefore := manager.ForkBoundaries()
	anchorsBefore := manager.AsideAnchors()

	asked := askAside(t, manager, "", "side?", "yes")

	assert.Equal(t, turnsBefore, manager.TurnBoundaries())
	assert.Equal(t, forksBefore, manager.ForkBoundaries())
	assert.Equal(t, anchorsBefore, manager.AsideAnchors())
	assert.NotContains(t, manager.RewindOffers(), asked.ID)

	forked, err := session.Fork(manager, "")
	require.NoError(t, err)
	path := forked.File()
	require.NoError(t, forked.Close())
	raw, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), asked.ID, "a fork copies the branch, and an aside is on no branch")
}

// The anchors offered to /btw are the messages of the context with a line
// that says what each one is.
func TestAsideAnchorsListTheMessagesOfTheContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")

	anchors := manager.AsideAnchors()
	require.Len(t, anchors, 4)
	assert.Equal(t, []string{turn.Prompt, turn.Call, turn.Result, turn.Answer},
		[]string{anchors[0].EntryID, anchors[1].EntryID, anchors[2].EntryID, anchors[3].EntryID})
	assert.Equal(t, "prompt one", anchors[0].Preview)
	assert.Equal(t, "answer looking it up", anchors[1].Preview)
	assert.Equal(t, "tool result file contents", anchors[2].Preview)
	assert.Equal(t, "answer answer one", anchors[3].Preview)
}

func TestAppendAsideRefusesWhatItCannotRecord(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	turn := recordTurn(t, manager, "one", "answer one")

	err := manager.AppendAside(session.AsideEntry{Question: "side?"})
	require.Error(t, err, "an aside without an id cannot be matched to its row")

	err = manager.AppendAside(session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: session.NewEntryID()},
		Question:         "  ",
	})
	require.ErrorIs(t, err, session.ErrEmptyAsideQuestion)

	err = manager.AppendAside(session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: turn.Answer},
		Question:         "side?",
	})
	require.ErrorContains(t, err, "already in the log")

	require.NoError(t, manager.Close())
	err = manager.AppendAside(session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: session.NewEntryID()},
		Question:         "side?",
	})
	require.ErrorIs(t, err, os.ErrClosed)
}

// An aside the file never received is not in memory either.
func TestAFailedAsideFlushLeavesNoTrace(t *testing.T) {
	dir := t.TempDir()
	manager, err := session.NewSessionManager(dir,
		session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	recordTurn(t, manager, "one", "answer one")
	require.NoError(t, os.Remove(manager.File()))
	require.NoError(t, os.Mkdir(manager.File(), 0o755))

	err = manager.AppendAside(session.AsideEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: session.NewEntryID()},
		Question:         "side?",
	})
	require.Error(t, err)
	assert.Empty(t, manager.Asides())
}

func entryIDs(entries []session.MessageEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.GetID())
	}
	return ids
}

// An aside draws as a row of its own kind, patched in place under its one id,
// and the next turn is a row of its own rather than a patch of the aside.
func TestAsideUpdateDrawsOneRowUnderItsID(t *testing.T) {
	snap := session.Apply(session.Snapshot{}, session.AsideUpdate{
		ID: "a1", Question: "side?", Answer: "part", State: session.StateStreaming,
		AnchorPreview: "answer earlier",
	})
	snap = session.Apply(snap, session.AsideUpdate{
		ID: "a1", Question: "side?", Answer: "partial answer", State: session.StateComplete,
		AnchorPreview: "answer earlier", SkippedTool: "bash", Model: "m",
	})
	require.Len(t, snap.Messages, 1)
	row := snap.Messages[0]
	assert.Equal(t, "a1", row.ID)
	assert.Equal(t, session.RoleAside, row.Role)
	assert.Equal(t, session.StateComplete, row.State)
	assert.Equal(t, "m", row.Model)
	assert.Equal(t, session.AsideRow{
		Question: "side?", AnchorPreview: "answer earlier", Answer: "partial answer", SkippedTool: "bash",
	}, row.Aside)

	items := session.Project(snap)
	require.Len(t, items, 1)
	assert.Equal(t, session.ItemAside, items[0].Kind)
	assert.Equal(t, row.Aside, items[0].Aside)

	snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
		ID: "turn", State: session.StateStreaming, Text: "hello",
	}})
	require.Len(t, snap.Messages, 2)
	assert.Equal(t, "partial answer", snap.Messages[0].Aside.Answer)
	assert.Equal(t, session.RoleAssistant, snap.Messages[1].Role)
}

// A streaming aside keeps the pipeline busy, the way a streaming turn does,
// so a local command is not started under it.
func TestAStreamingAsideCountsAsStreaming(t *testing.T) {
	snap := session.Apply(session.Snapshot{}, session.AsideUpdate{
		ID: "a1", Question: "side?", State: session.StateStreaming,
	})
	assert.True(t, session.IsStreaming(snap))
	snap = session.Apply(snap, session.AsideUpdate{ID: "a1", Question: "side?", State: session.StateComplete})
	assert.False(t, session.IsStreaming(snap))
}

func anchorPreviewOf(t *testing.T, manager *session.Manager, entryID string) string {
	t.Helper()
	for _, anchor := range manager.AsideAnchors() {
		if anchor.EntryID == entryID {
			return anchor.Preview
		}
	}
	t.Fatalf("no anchor %s", entryID)
	return ""
}

// A question about the end of the context is a question about all of it and
// names nothing; one about an earlier message names that message.
func TestAsideScopeNamesAnAnchorThatIsNotTheEndOfTheContext(t *testing.T) {
	manager := session.NewManager(t.TempDir())
	first := recordTurn(t, manager, "one", "answer one")
	second := recordTurn(t, manager, "two", "answer two")

	whole, err := manager.AsideScope("")
	require.NoError(t, err)
	assert.Empty(t, whole.AnchorPreview)

	atEnd, err := manager.AsideScope(second.Answer)
	require.NoError(t, err)
	assert.Empty(t, atEnd.AnchorPreview, "the last message is the whole context")

	earlier, err := manager.AsideScope(first.Answer)
	require.NoError(t, err)
	assert.Equal(t, anchorPreviewOf(t, manager, first.Answer), earlier.AnchorPreview)
	assert.NotEmpty(t, earlier.AnchorPreview)
}
