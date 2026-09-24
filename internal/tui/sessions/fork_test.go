package sessions

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// forkTabs stands in for the shell: it says whether another tab may be
// opened, opens a session file as another view exactly as the tab registry
// does, and keeps what it opened so the test can look at the tab the fork
// landed in. full and broken make it answer the way a shell out of room and
// a shell that fails to build a tab answer.
type forkTabs struct {
	t      *testing.T
	cwd    string
	opened []*View
	full   error
	broken error
}

func newForkTabs(t *testing.T, cwd string) *forkTabs {
	t.Helper()
	return &forkTabs{t: t, cwd: cwd}
}

func (f *forkTabs) room() error { return f.full }

func (f *forkTabs) open(path string) (*View, error) {
	if f.broken != nil {
		return nil, f.broken
	}
	f.t.Helper()
	proj, err := project.Discover(f.cwd)
	if err != nil {
		return nil, err
	}
	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, f.cwd, path)
	if err != nil {
		return nil, err
	}
	f.t.Cleanup(ctrl.Close)
	view := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), f.cwd, "m", nil, 1000, nil, nil)
	f.opened = append(f.opened, view)
	return view, nil
}

func (f *forkTabs) last() *View {
	f.t.Helper()
	require.NotEmpty(f.t, f.opened, "no tab was opened")
	return f.opened[len(f.opened)-1]
}

// The whole fork, through the real shell: two turns, a fork at the first
// answer, and a tab that opens on the copy. The new tab carries the history
// up to the anchor, and the session it was taken from is untouched.
func TestForkOpensATabWithTheHistoryUpToTheAnchor(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	snap := e.transcript.Snapshot()
	anchor := snap.Messages[1].ID // the answer that finished the first turn
	require.NotEmpty(t, anchor)
	e.composer.Chat.Value = "a note to myself"

	e.ForkFrom(anchor)

	forked := tabs.last()
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(forked.transcript.Snapshot()),
		"the new tab holds the conversation up to the anchor")
	assert.NotEqual(t, ctrl.SessionID(), forked.ctrl.SessionID(), "the copy is a session of its own")
	assert.NotEqual(t, ctrl.SessionFile(), forked.ctrl.SessionFile())

	assert.Equal(t, []string{"first", "reply 1", "second", "reply 2"}, messageTexts(e.transcript.Snapshot()),
		"the session forked from keeps its whole transcript")
	assert.Equal(t, "a note to myself", e.composer.Chat.Value, "and its draft")

	history := forked.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "Forked into",
		"the tab the fork landed in says so")
}

// A turn taken in the fork is the fork's own. The session it came from does
// not hear about it, on screen or in its file.
func TestATurnInTheForkedTabStaysThere(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	e.ForkFrom("")
	forked := tabs.last()
	require.Equal(t, []string{"first", "reply 1"}, messageTexts(forked.transcript.Snapshot()))

	runTurn(t, forked, "only here", 4)
	assert.Equal(t, []string{"first", "reply 1", "only here", "reply 2"},
		messageTexts(forked.transcript.Snapshot()))
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(e.transcript.Snapshot()),
		"the tab forked from is where it was")
}

// A fork cut before a prompt hands its text to the composer of the new tab,
// so the question can be asked again, differently, without the answer it got
// the first time in the context.
func TestForkBeforeAPromptFillsTheNewTabsComposer(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	e.ForkFrom(e.transcript.Snapshot().Messages[2].ID)

	forked := tabs.last()
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(forked.transcript.Snapshot()))
	assert.Equal(t, "second", forked.composer.Chat.Value,
		"the prompt the copy stopped before is waiting in the new tab")
	assert.Empty(t, e.composer.Chat.Value, "and it was not taken out of the tab it came from")
}

// /fork with no id copies the conversation as it stands, and /fork <id> does
// what the button on that row does. The two are one operation.
func TestSlashForkCopiesTheWholeConversation(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	require.True(t, e.commands.DispatchSlash("/fork", e.commandContext()))
	assert.Equal(t, []string{"first", "reply 1", "second", "reply 2"},
		messageTexts(tabs.last().transcript.Snapshot()),
		"no id means the whole conversation")

	anchor := e.transcript.Snapshot().Messages[1].ID
	require.True(t, e.commands.DispatchSlash("/fork "+anchor, e.commandContext()))
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(tabs.last().transcript.Snapshot()),
		"an id means the branch up to it, the same as the button on that row")
}

// The completer offers every boundary, newest first, the one the cursor
// stands on included: copying the whole conversation is a fork like any
// other, and the rewind list is the one that leaves it out.
func TestForkCompleterListsEveryBoundary(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	items, ok := e.commands.CompleteSlashArg("fork", nil, "")
	require.True(t, ok)
	require.Len(t, items, 4)
	assert.Equal(t, "after reply 2", items[0].Description)
	assert.Equal(t, "before second", items[1].Description)
	assert.Equal(t, "after reply 1", items[2].Description)
	assert.Equal(t, "before first", items[3].Description)

	lastAnswer := e.transcript.Snapshot().Messages[3].ID
	assert.Equal(t, lastAnswer, items[0].Path)

	only, ok := e.commands.CompleteSlashArg("fork", nil, lastAnswer)
	require.True(t, ok)
	require.Len(t, only, 1)
	assert.Equal(t, lastAnswer, only[0].Path)
}

// A fork asked for at an id this session never recorded is refused out loud,
// and nothing is opened and nothing is written.
func TestForkAtAnUnknownEntryIsRefused(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	e.ForkFrom("no-such-entry")

	assert.Empty(t, tabs.opened, "a refused fork opens no tab")
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "Cannot fork")
	assert.Contains(t, history[len(history)-1].Message, "no-such-entry")
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(e.transcript.Snapshot()))
}

// A screen the shell never gave a tab opener to says so instead of writing a
// copy nobody can reach. A sub-agent's screen is the one this happens on.
func TestForkOnAScreenWithoutTabsIsRefused(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	before := sessionFiles(t, ctrl.SessionDir())

	e.ForkFrom("")

	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "does not open tabs")
	assert.Equal(t, before, sessionFiles(t, ctrl.SessionDir()),
		"and no copy was written for nobody to open")
}

// sessionFiles lists the session logs of a project directory.
func sessionFiles(t *testing.T, dir string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	slices.Sort(found)
	return found
}

// The shell is asked whether a fork has somewhere to land before anything is
// written. With every tab taken the answer is no, and the session directory
// is left exactly as it was: a copy for a tab that cannot open is a file
// nobody asked for.
func TestForkAsksForATabBeforeWritingTheCopy(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	tabs.full = errors.New("session limit (12) reached: close a session before opening another")
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	before := sessionFiles(t, ctrl.SessionDir())

	e.ForkFrom("")

	assert.Empty(t, tabs.opened)
	assert.Equal(t, before, sessionFiles(t, ctrl.SessionDir()),
		"no copy was written for a tab that cannot be opened")
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "Cannot fork")
	assert.Contains(t, history[len(history)-1].Message, "session limit (12) reached",
		"the refusal is the shell's own words")
}

// A tab that had room and still failed to open leaves the copy on disk. The
// answer says so and names the session, because /resume is how it is reached.
func TestForkThatCannotOpenATabNamesTheCopy(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	tabs.broken = errors.New("the terminal is too small for another session")
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	before := sessionFiles(t, ctrl.SessionDir())

	e.ForkFrom("")

	after := sessionFiles(t, ctrl.SessionDir())
	require.Len(t, after, len(before)+1, "the copy was written before the tab was asked for")
	history := e.toast.History()
	require.NotEmpty(t, history)
	last := history[len(history)-1].Message
	assert.Contains(t, last, "no tab could be opened")
	assert.Contains(t, last, "the terminal is too small for another session")
	assert.Contains(t, last, "/resume ", "the copy is on disk, so the answer says how to reach it")
}
