package sessions

import (
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// openContextAt opens the browser and walks its cursor up from the newest
// row to the one standing for the entry, the way a hand on the keyboard
// would.
func openContextAt(t *testing.T, e *View, entryID string) {
	t.Helper()
	e.ShowContext()
	require.True(t, e.ctxpane.Visible())
	items := e.ctrl.ContextView().Items
	row := -1
	for i, item := range items {
		if item.EntryID == entryID {
			row = i
		}
	}
	require.NotEqual(t, -1, row, "the browser lists the entry")
	for range len(items) - 1 - row {
		e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyUp})
	}
}

// The browser reads the same offers the feed's strip does: every prompt and
// every finished reply but the newest is a rewind, every one of them is a
// fork, and an idle shell is not busy.
func TestContextBrowserOffersMatchTheEngine(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	msgs := e.transcript.Snapshot().Messages

	offers := e.contextActionOffers()
	assert.False(t, offers.Busy)
	for i, msg := range msgs {
		assert.Contains(t, offers.Fork, msg.ID, "row %d forks", i)
		_, rewinds := offers.Rewind[msg.ID]
		assert.Equal(t, i != len(msgs)-1, rewinds, "row %d rewinds unless it is the newest reply", i)
	}
}

// r on a prompt in the browser is the same cut the strip's button takes: the
// rows past it leave the feed, the prompt comes back to the composer and the
// browser is gone.
func TestContextBrowserRewindReachesTheShell(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[2].ID

	openContextAt(t, e, anchor)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'r'})

	assert.False(t, e.ctxpane.Visible(), "the browser closes before the cut")
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(e.transcript.Snapshot()))
	assert.Equal(t, "second", e.composer.Chat.Value)
}

// b on a reply in the browser leaves the composer in btw mode anchored at
// that reply, which needs the browser to have handed the keyboard back.
func TestContextBrowserBtwEntersAsideMode(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[1].ID

	openContextAt(t, e, anchor)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'b'})

	assert.False(t, e.ctxpane.Visible())
	assert.Contains(t, e.composer.Chat.AgentLabel.Text, "btw @"+anchor)
}

// While a turn runs the browser's offers say busy, and r on a prompt refuses
// into the footer instead of cutting under a stream that is still writing.
func TestContextBrowserRefusesARewindWhileATurnRuns(t *testing.T) {
	srv, _, firstStarted, release := queuedSSEServer(t)
	defer srv.Close()
	defer release()
	e, ctrl := newQueueEditor(t, srv.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	submitPrompt(e, "first")
	waitFor(t, 5*time.Second, func() bool {
		select {
		case <-firstStarted:
			return true
		default:
			return false
		}
	})
	waitFor(t, 5*time.Second, func() bool {
		e.DrainNow()
		snap := e.transcript.Snapshot()
		return len(snap.Messages) >= 2 && snap.Messages[1].Role == session.RoleAssistant
	})
	require.True(t, ctrl.RunActive(), "the first turn is still streaming")
	assert.True(t, e.contextActionOffers().Busy, "the browser is told the shell is busy")

	prompt := e.transcript.Snapshot().Messages[0].ID
	openContextAt(t, e, prompt)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'r'})

	assert.True(t, e.ctxpane.Visible(), "a refusal keeps the browser open")
	assert.Contains(t, viewText(t, e), "a turn is running", "and says why in the footer")
	assert.Len(t, e.transcript.Snapshot().Messages, 2, "the feed was not cut")

	e.ctxpane.Hide()
	release()
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		return !ctrl.RunActive()
	})
}

// f on a boundary in the browser is the fork the strip's button takes: a
// tab opens on the copy up to that row and the browser is gone.
func TestContextBrowserForkReachesTheShell(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()
	cwd := t.TempDir()
	e, ctrl := newQueueEditor(t, server.URL, cwd)
	t.Cleanup(ctrl.Close)
	tabs := newForkTabs(t, cwd)
	e.ConfigureSessionFork(tabs.room, tabs.open)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[1].ID

	openContextAt(t, e, anchor)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'f'})

	assert.False(t, e.ctxpane.Visible())
	forked := tabs.last()
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(forked.transcript.Snapshot()),
		"the new tab holds the conversation up to the row")
	assert.Equal(t, []string{"first", "reply 1", "second", "reply 2"}, messageTexts(e.transcript.Snapshot()),
		"the session forked from is untouched")
}
