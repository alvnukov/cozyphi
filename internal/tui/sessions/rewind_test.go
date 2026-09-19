package sessions

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/session"
)

// replyingSSEServer answers every request with one assistant line and keeps
// the request bodies, so a test can prove what the model was and was not
// shown after a rewind.
func replyingSSEServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var recorded []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		n := len(recorded)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta(fmt.Sprintf("reply %d", n)))
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	bodies := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
	return server, bodies
}

// runTurn submits a prompt and waits for the turn to be finished, not merely
// quiet. The last chunk reaching the feed is not the end of it: the loop
// yields the complete event before it appends the answer to the session, and
// a queued prompt keeps the pipeline busy after that. RunActive covers both,
// so waiting on it is waiting until the log has settled.
func runTurn(t *testing.T, e *View, text string, want int) {
	t.Helper()
	submitPrompt(e, text)
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		snap := e.transcript.Snapshot()
		return len(snap.Messages) >= want && !session.IsStreaming(snap) && !e.ctrl.RunActive()
	})
}

func messageTexts(snap session.Snapshot) []string {
	out := make([]string, 0, len(snap.Messages))
	for _, msg := range snap.Messages {
		out = append(out, msg.FlatText())
	}
	return out
}

// The whole rewind, through the real shell: two turns, a cut before the
// second prompt, and then a reworded second turn. The feed loses the rows
// past the cut, the prompt comes back to the composer, and the model's next
// request carries the first turn and the new prompt but nothing of the turn
// that was cut away.
func TestRewindCutsTheFeedAndTheModelsNextRequest(t *testing.T) {
	server, bodies := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	snap := e.transcript.Snapshot()
	require.Equal(t, []string{"first", "reply 1", "second", "reply 2"}, messageTexts(snap))
	anchor := snap.Messages[2].ID
	require.NotEmpty(t, anchor)

	e.RewindTo(anchor)

	snap = e.transcript.Snapshot()
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(snap),
		"the rows past the cut left the feed")
	assert.Equal(t, "second", e.composer.Chat.Value,
		"the prompt came back so it can be sent again, reworded")
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "/rewind back")

	runTurn(t, e, "second, reworded", 4)
	assert.Equal(t, []string{"first", "reply 1", "second, reworded", "reply 3"},
		messageTexts(e.transcript.Snapshot()))

	got := bodies()
	require.Len(t, got, 3)
	assert.Contains(t, got[2], "second, reworded")
	assert.Contains(t, got[2], "first", "the turn before the cut is still the model's history")
	assert.Contains(t, got[2], "reply 1", "and so is the answer that finished it")
	assert.NotContains(t, got[2], "reply 2",
		"the answer on the branch the cursor left is not shown to the model again")
}

// Undo puts the feed back on the branch the rewind left, and takes back the
// prompt it handed over, because that prompt is part of the context again.
func TestRewindBackRestoresTheEarlierView(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	anchor := e.transcript.Snapshot().Messages[2].ID

	e.RewindTo(anchor)
	require.Equal(t, "second", e.composer.Chat.Value)

	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))

	assert.Equal(t, []string{"first", "reply 1", "second", "reply 2"},
		messageTexts(e.transcript.Snapshot()))
	assert.Empty(t, e.composer.Chat.Value,
		"the prompt is back in the context, so it is taken out of the composer")
}

// A draft the user edited after the rewind is theirs: undoing the rewind must
// not wipe it, even though the prompt it was appended to is going back.
func TestRewindBackLeavesAnEditedDraftAlone(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	e.RewindTo(e.transcript.Snapshot().Messages[2].ID)

	e.composer.Chat.Value = "second, but differently"
	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	assert.Equal(t, "second, but differently", e.composer.Chat.Value)
}

// A draft the user had typed BEFORE the rewind is theirs too. The undo takes
// back the prompt it handed over and puts the composer back as it found it,
// rather than emptying it.
func TestRewindBackKeepsTheDraftTypedBeforeIt(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	e.composer.Chat.Value = "a note to myself"
	e.RewindTo(e.transcript.Snapshot().Messages[2].ID)
	require.Equal(t, "a note to myself\nsecond", e.composer.Chat.Value,
		"the prompt joins the draft instead of replacing it")

	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	assert.Equal(t, "a note to myself", e.composer.Chat.Value,
		"only the prompt the rewind handed over is taken back")
}

// Two rewinds in a row stack two prompts in the composer, and one undo takes
// back one of them. Undoing one move must not empty what the other handed
// over: that prompt is still out of the context and still the user's to send.
func TestOneUndoTakesBackOneHandedOverPrompt(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	snap := e.transcript.Snapshot()
	e.RewindTo(snap.Messages[2].ID)
	require.Equal(t, "second", e.composer.Chat.Value)
	e.RewindTo(snap.Messages[0].ID)
	require.Equal(t, "second\nfirst", e.composer.Chat.Value)

	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	assert.Equal(t, "second", e.composer.Chat.Value,
		"the undo took back the second hand-back and left the first alone")
}

// A cut asked for at an id this session never recorded is refused out loud,
// and the feed is left exactly as it was. Whether a cut is refused while a
// turn runs is the controller's own question, pinned where the answer lives.
func TestRewindAtAnUnknownEntryIsRefusedAndChangesNothing(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	before := messageTexts(e.transcript.Snapshot())

	e.RewindTo("no-such-entry")

	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "Cannot rewind")
	assert.Contains(t, history[len(history)-1].Message, "no-such-entry")
	assert.Equal(t, before, messageTexts(e.transcript.Snapshot()))
}

// The completer offers the way out first and then the boundaries, newest
// first, each with the line that says what a cut there would do.
func TestRewindCompleterListsTheBoundaries(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)

	items, ok := e.commands.CompleteSlashArg("rewind", nil, "")
	require.True(t, ok)
	require.Len(t, items, 5)
	assert.Equal(t, "back", items[0].Path)
	assert.Equal(t, "after reply 2", items[1].Description)
	assert.Equal(t, "before second", items[2].Description)
	assert.Equal(t, "after reply 1", items[3].Description)
	assert.Equal(t, "before first", items[4].Description)

	only, ok := e.commands.CompleteSlashArg("rewind", nil, "ba")
	require.True(t, ok)
	require.Len(t, only, 1)
	assert.Equal(t, "back", only[0].Path)
}

// /rewind with nothing to go on says how to use it rather than guessing.
func TestRewindWithoutAnArgumentExplainsItself(t *testing.T) {
	e := newActionsEditor(t)
	require.True(t, e.commands.DispatchSlash("/rewind", e.commandContext()))
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "/rewind back")
}

// The undo with nothing behind it reaches the user as a notice, not silence.
func TestRewindBackWithoutARewindSaysSo(t *testing.T) {
	e := newActionsEditor(t)
	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "nothing to undo")
}

// The strips guard themselves while a turn runs, and the guard has to come
// off when the turn ends. The turn's last event reaches the feed while the
// pipeline is still busy, so whatever that event baked in outlives the turn
// unless the end of the run rebuilds the rows. Nothing else does.
func TestTheStripActsAgainOnceTheTurnIsOver(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)
	e.App = app.NewApp(nil, components.DefaultTheme())

	runTurn(t, e, "first", 2)

	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		return e.transcript.MessageActionsGuard() == ""
	})
	assert.Empty(t, e.transcript.MessageActionsGuard(),
		"the run is over, so the buttons may act")

	// And the click really acts: the whole path from the drawn button to the
	// engine, on a row the session has actually recorded.
	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 140, Height: 40},
		Method: xui.WidthUnicode,
	})
	x, y, ok := controlTextPosition(root, nil, "rewind", components.Point{})
	require.True(t, ok, "the prompt draws a rewind button")
	hit, lx, ly := root.HitTestAt(x+1, y)
	require.NotNil(t, hit)
	hit.Handle(&components.EventContext{},
		xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MousePress})
	hit.Handle(&components.EventContext{},
		xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MouseRelease})

	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[len(history)-1].Message, "Rewound to",
		"the click rewound rather than being refused")
	assert.Empty(t, messageTexts(e.transcript.Snapshot()),
		"the cut was before the only prompt, so the context is empty")
}

// A cut at an answer hands no prompt over, and it must not be mistaken for an
// undo. Rewinding to a prompt and then further back to an answer used to wipe
// the text the first cut handed over, at a moment when the feed no longer
// held it either.
func TestASecondCutAtAnAnswerKeepsTheFirstCutsPrompt(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	runTurn(t, e, "third", 6)

	snap := e.transcript.Snapshot()
	e.RewindTo(snap.Messages[4].ID) // before the third prompt
	require.Equal(t, "third", e.composer.Chat.Value)

	e.RewindTo(snap.Messages[1].ID) // after the first answer
	assert.Equal(t, "third", e.composer.Chat.Value,
		"a cut that hands nothing over takes nothing away either")
	assert.Equal(t, []string{"first", "reply 1"}, messageTexts(e.transcript.Snapshot()))

	// The undo still takes back what the first cut handed over, once it is
	// the move being undone.
	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	assert.Equal(t, "third", e.composer.Chat.Value,
		"this undo reverses the answer cut, which handed nothing over")
}

// The draft belongs to the session it was typed in. Carried across a switch,
// the undo would cut text out of the composer that this session never put
// there.
func TestASessionSwitchForgetsWhatTheUndoWouldTakeBack(t *testing.T) {
	server, _ := replyingSSEServer(t)
	defer server.Close()

	e, ctrl := newQueueEditor(t, server.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	runTurn(t, e, "first", 2)
	runTurn(t, e, "second", 4)
	e.RewindTo(e.transcript.Snapshot().Messages[2].ID)
	require.Equal(t, "second", e.composer.Chat.Value)

	e.ClearSession()
	e.composer.Chat.Value = "second"

	require.True(t, e.commands.DispatchSlash("/rewind back", e.commandContext()))
	assert.Equal(t, "second", e.composer.Chat.Value,
		"the new session's undo has nothing of its own to take back")
}
