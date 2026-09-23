package sessions

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// waitForAside waits until the side question has been answered and the
// pipeline it held is free again.
func waitForAside(t *testing.T, e *View) {
	t.Helper()
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		snap := e.transcript.Snapshot()
		if len(snap.Messages) == 0 {
			return false
		}
		last := snap.Messages[len(snap.Messages)-1]
		return strings.HasPrefix(last.Text, "btw:") && last.State != session.StateStreaming && !e.ctrl.RunActive()
	})
}

// The whole side question, through the real shell: /btw is typed into the
// composer, the answer lands in the feed as a row of its own, and the turn
// after it is sent without the question or the answer.
func TestSlashBtwAnswersAndTheNextTurnNeverSeesIt(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	submitPrompt(e, "/btw what did we talk about?")
	waitForAside(t, e)

	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 5)
	assert.Equal(t, "btw: what did we talk about?\n\nreply 3", snap.Messages[4].Text)
	assert.Equal(t, session.StateComplete, snap.Messages[4].State)
	got := bodies()
	require.Len(t, got, 3)
	assert.Contains(t, got[2], "reply 2", "the side question is asked about the whole context")

	runTurn(t, e, "third", 7)
	got = bodies()
	require.Len(t, got, 4)
	assert.Contains(t, got[3], "third")
	assert.NotContains(t, got[3], "what did we talk about?", "the next turn does not carry the question")
	assert.NotContains(t, got[3], "reply 3", "nor the answer")
}

// /btw @<id> asks about the context as it stood at that message: what came
// after it is not sent.
func TestSlashBtwAtAnAnchorAsksAboutTheContextUpToIt(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[1].ID
	require.NotEmpty(t, anchor)

	require.True(t, e.commands.DispatchSlash("/btw @"+anchor+" why so?", e.commandContext()))
	waitForAside(t, e)

	got := bodies()
	require.Len(t, got, 3)
	assert.Contains(t, got[2], "reply 1")
	assert.Contains(t, got[2], "why so?")
	assert.NotContains(t, got[2], "reply 2", "nothing after the anchor is sent")
}

// The completer offers the messages of the context behind an @, newest
// first, and leaves a question alone.
func TestSlashBtwCompletesAnchorsBehindAnAt(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)
	runTurn(t, e, "first", 2)
	ids := []string{e.transcript.Snapshot().Messages[0].ID, e.transcript.Snapshot().Messages[1].ID}

	items, ok := e.commands.CompleteSlashArg("btw", nil, "@")
	require.True(t, ok)
	require.Len(t, items, 2)
	assert.Equal(t, "@"+ids[1], items[0].Path, "the newest message comes first")
	assert.Equal(t, "answer reply 1", items[0].Description)
	assert.Equal(t, "@"+ids[0], items[1].Path)

	items, ok = e.commands.CompleteSlashArg("btw", nil, "@"+ids[0][:4])
	require.True(t, ok)
	assert.NotEmpty(t, items)

	_, ok = e.commands.CompleteSlashArg("btw", nil, "what")
	assert.False(t, ok, "a word of the question is not completed")
	_, ok = e.commands.CompleteSlashArg("btw", []string{"@" + ids[0]}, "@")
	assert.False(t, ok, "only the first argument names an anchor")
}

// A question that cannot be asked is refused out loud and asks nothing.
func TestSlashBtwRefusalsAreShown(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)
	runTurn(t, e, "first", 2)

	for _, line := range []string{"/btw @", "/btw @nothing-like-this why?"} {
		require.True(t, e.commands.DispatchSlash(line, e.commandContext()), line)
		history := e.toast.History()
		require.NotEmpty(t, history, line)
		last := history[len(history)-1].Message
		assert.True(t, strings.Contains(last, "/btw") || strings.Contains(last, "Cannot ask on the side"), last)
	}
	assert.Len(t, bodies(), 1, "only the turn reached the model")
	assert.Len(t, e.transcript.Snapshot().Messages, 2, "no refusal drew a row")
}

func TestBareSlashAndPaletteBtwEnterCurrentContextMode(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()
	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	require.True(t, e.commands.DispatchSlash("/btw", e.commandContext()))
	require.Equal(t, "⏵⏵ btw", e.composer.Chat.AgentLabel.Text)
	e.composer.LeaveAside()

	for _, item := range e.commands.BuildPalette(e.commandContext()) {
		if item.ID == "btw" {
			require.Equal(t, "Ctrl+T", item.Shortcut)
			item.Run()
			require.Equal(t, "⏵⏵ btw", e.composer.Chat.AgentLabel.Text)
			return
		}
	}
	t.Fatal("btw command missing from palette")
}

// Esc stops a side question the way it stops a reply: the row says the answer
// was cancelled, nothing is written to the file, and the pipeline is free.
func TestEscCancelsASideQuestionAndLeavesNoRecord(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("partial"))
		if !strings.Contains(string(body), "side question") {
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)
	runTurn(t, e, "first", 2)

	submitPrompt(e, "/btw still there?")
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		snap := e.transcript.Snapshot()
		return len(snap.Messages) == 3 && strings.Contains(snap.Messages[2].Text, "partial")
	})

	require.True(t, e.interruptWork(), "Esc found the side question in flight")
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		return !e.ctrl.RunActive()
	})
	e.DrainNow()

	row := e.transcript.Snapshot().Messages[2]
	assert.True(t, strings.HasPrefix(row.Text, "btw: still there?"))
	assert.Equal(t, session.StateCancelled, row.State)
	raw, err := os.ReadFile(ctrl.SessionFile())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"type":"aside"`, "a cancelled answer is not written down")
}
