package sessions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// queuedSSEServer streams an OpenAI-compatible reply. The first request writes
// a partial chunk and then blocks until released, so the test can submit a
// second prompt while the first turn is genuinely mid-stream. It records each
// request body so the test can prove both prompts reached the model, in order.
func queuedSSEServer(
	t *testing.T,
) (srv *httptest.Server, bodies func() []string, firstStarted <-chan struct{}, release func()) {
	t.Helper()
	firstStartedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(releaseCh) }) }

	var mu sync.Mutex
	var recorded []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		n := len(recorded)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		if n == 1 {
			// A partial assistant chunk proves the first turn is streaming,
			// not just queued server-side.
			_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("first "))
			if flusher != nil {
				flusher.Flush()
			}
			close(firstStartedCh)
			select {
			case <-releaseCh:
			case <-r.Context().Done():
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("reply"))
		} else {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("second reply"))
		}
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))

	bodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
	return srv, bodies, firstStartedCh, release
}

func sseDelta(content string) string {
	return `{"choices":[{"delta":{"role":"assistant","content":"` + content + `"}}]}`
}

func newQueueEditor(t *testing.T, baseURL, cwd string) (*View, *controller.Controller) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", baseURL)

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)

	e := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil)
	return e, ctrl
}

// submitPrompt drives a full user submit through the editor's key handling:
// text lands in the composer, Enter reaches ChatInput, and the submitter
// appends the user row and hands the prompt to the controller.
func submitPrompt(e *View, text string) {
	e.composer.Chat.Value = text
	e.composer.Chat.Cursor = len(text)
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestEditorQueuedSubmitReachesModel is the UI-level integration test for the
// queue: a prompt submitted while the model is answering stays OUT of the
// transcript (it waits in the composer queue widget), and when the in-flight
// turn finishes the engine delivers it — its row lands at the END of the
// transcript, followed by the answer. All through View.Handle, the bus, and
// the transcript projection, with a real streaming fake model server.
func TestEditorQueuedSubmitReachesModel(t *testing.T) {
	srv, bodies, firstStarted, release := queuedSSEServer(t)
	defer srv.Close()
	defer release()

	e, ctrl := newQueueEditor(t, srv.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	// First prompt starts a run and the model begins streaming.
	submitPrompt(e, "first")
	waitFor(t, 5*time.Second, func() bool {
		select {
		case <-firstStarted:
			return true
		default:
			return false
		}
	})

	// Let the streaming assistant row reach the transcript before the second
	// submit, so the queue really fires mid-stream in the UI too.
	waitFor(t, 5*time.Second, func() bool {
		e.DrainNow()
		snap := e.transcript.Snapshot()
		return len(snap.Messages) >= 2 && snap.Messages[1].Role == session.RoleAssistant
	})

	// Submit while the model is still answering: the prompt waits in the
	// queue widget, no transcript row yet.
	submitPrompt(e, "second")
	e.DrainNow()

	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 2, "queued prompt must not enter the transcript")
	require.Equal(t, 0, countUserRows(snap, "second"))
	require.True(t, ctrl.RunActive(), "queued prompt keeps the pipeline busy")

	// Finish the first turn: the controller dequeues the second prompt, the
	// engine delivers it, and its row is appended at the end.
	release()

	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		s := e.transcript.Snapshot()
		return len(s.Messages) >= 4 && !session.IsStreaming(s)
	})

	snap = e.transcript.Snapshot()
	require.Len(t, snap.Messages, 4, "two turns => user, assistant, user, assistant")
	assert.Equal(t, session.RoleUser, snap.Messages[0].Role)
	assert.Equal(t, session.RoleAssistant, snap.Messages[1].Role)
	assert.Equal(t, "first reply", snap.Messages[1].FlatText())
	assert.Equal(t, session.RoleUser, snap.Messages[2].Role)
	assert.Equal(t, "second", snap.Messages[2].Text)
	assert.Equal(t, session.RoleAssistant, snap.Messages[3].Role)
	assert.Equal(t, "second reply", snap.Messages[3].FlatText())

	// Both prompts reached the model, in submission order.
	got := bodies()
	require.Len(t, got, 2, "model must receive exactly two requests")
	assert.Contains(t, got[0], "first", "first request must carry the first prompt")
	assert.Contains(t, got[1], "second", "second request must carry the queued prompt")
}

// inPlaceSSEServer: request 1 streams partial text, blocks mid-stream (so the
// test can queue a follow-up while the turn is genuinely streaming), then
// finishes round 1 with a read tool call. Round 2 — after the tool-round
// boundary injects the queued prompt — answers with plain text.
func inPlaceSSEServer(
	t *testing.T,
	readFile string,
) (srv *httptest.Server, bodies func() []string, firstStarted <-chan struct{}, release func()) {
	t.Helper()
	firstStartedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(releaseCh) }) }

	var mu sync.Mutex
	var recorded []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = append(recorded, string(body))
		n := len(recorded)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		if n == 1 {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("round one "))
			if flusher != nil {
				flusher.Flush()
			}
			close(firstStartedCh)
			select {
			case <-releaseCh:
			case <-r.Context().Done():
				return
			}
			args, _ := json.Marshal(map[string]string{"path": readFile})
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []any{map[string]any{
							"index": 0, "id": "call_1", "type": "function",
							"function": map[string]any{"name": "read", "arguments": string(args)},
						}},
					},
				}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		} else {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", sseDelta("answered in round two"))
		}
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))

	bodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
	return srv, bodies, firstStartedCh, release
}

// TestEditorQueuedRowAppendedAtDelivery pins the transcript semantics the
// user asked for: a prompt submitted mid-run has NO transcript row while it
// waits — it hangs in the queue widget above the input. The row appears at
// the END of the transcript at the moment the model receives the message
// (the tool-round boundary), so the final order is: round 1, the user row,
// then the model's answer to it.
func TestEditorQueuedRowAppendedAtDelivery(t *testing.T) {
	cwd := t.TempDir()
	readFile := filepath.Join(cwd, "note.txt")
	require.NoError(t, os.WriteFile(readFile, []byte("hello"), 0o644))

	srv, bodies, firstStarted, release := inPlaceSSEServer(t, readFile)
	defer srv.Close()
	defer release()

	e, ctrl := newQueueEditor(t, srv.URL, cwd)
	ctrl.SetAllowAll(true)
	// Build mode: the plan gate must not reject the round's read call —
	// the test wants a genuine successful tool round before the boundary.
	ctrl.SetMode(agent.ModeBuild)
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

	// Submit while round 1 is mid-stream: no transcript row — the prompt
	// waits in the queue widget above the input.
	submitPrompt(e, "hold my place")
	e.DrainNow()
	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 2, "queued prompt must not enter the transcript")
	require.Equal(t, 0, countUserRows(snap, "hold my place"))

	// Release round 1: the tool runs, the boundary delivers the queued row
	// mid-turn, and round 2 answers it below. Once the row appears it must
	// sit at the end (only the answering assistant may follow it), keep its
	// id, and never duplicate.
	release()
	queuedID := ""
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		s := e.transcript.Snapshot()
		n := countUserRows(s, "hold my place")
		require.LessOrEqual(t, n, 1, "the queued row must not be duplicated by the mid-turn injection")
		if n == 1 {
			idx := len(s.Messages) - 1
			if s.Messages[idx].Text != "hold my place" {
				idx--
			}
			row := s.Messages[idx]
			require.Equal(t, "hold my place", row.Text)
			require.Equal(t, session.RoleUser, row.Role)
			if queuedID == "" {
				queuedID = row.ID
			} else {
				require.Equal(t, queuedID, row.ID, "the delivered row must keep its id")
			}
		}
		return len(s.Messages) >= 4 && !session.IsStreaming(s)
	})

	snap = e.transcript.Snapshot()
	require.Len(t, snap.Messages, 4, "round 1, delivered row, round 2")
	assert.Equal(t, session.RoleAssistant, snap.Messages[1].Role)
	assert.Equal(t, "round one ", snap.Messages[1].FlatText())
	assert.Equal(t, session.RoleUser, snap.Messages[2].Role)
	assert.Equal(t, "hold my place", snap.Messages[2].Text)
	assert.Equal(t, queuedID, snap.Messages[2].ID, "same row the engine promoted")
	assert.Equal(t, session.RoleAssistant, snap.Messages[3].Role)
	assert.Equal(t, "answered in round two", snap.Messages[3].FlatText())

	got := bodies()
	require.Len(t, got, 2, "mid-turn injection must reuse the running turn")
	assert.Contains(t, got[1], "hold my place", "round 2 must already carry the queued row")
}

func countUserRows(s session.Snapshot, text string) int {
	n := 0
	for _, m := range s.Messages {
		if m.Role == session.RoleUser && m.Text == text {
			n++
		}
	}
	return n
}

// TestEditorEscRecallsQueuedPrompt is the UI-level contract for Esc recall:
// while the first turn streams and a second prompt sits queued, Esc pulls
// the newest queued message back into the composer — the transcript never
// held a row for it — and leaves the run alone: the model never sees the
// recalled prompt, and the input is submittable again once the turn ends.
func TestEditorEscRecallsQueuedPrompt(t *testing.T) {
	srv, bodies, firstStarted, release := queuedSSEServer(t)
	defer srv.Close()
	defer release()

	e, ctrl := newQueueEditor(t, srv.URL, t.TempDir())
	t.Cleanup(ctrl.Close)

	// First prompt starts a run and the model begins streaming.
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

	// Submit while the model is still answering: the prompt waits in the
	// queue widget — no transcript row yet.
	submitPrompt(e, "second")
	e.DrainNow()
	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 2, "queued prompt must not enter the transcript")
	require.Equal(t, 0, countUserRows(snap, "second"))

	// Esc recalls: the text returns to the input, the transcript is untouched.
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	e.DrainNow()

	snap = e.transcript.Snapshot()
	require.Equal(t, 0, countUserRows(snap, "second"), "recall must not add a row")
	require.Len(t, snap.Messages, 2, "only the first turn's rows remain")
	assert.Contains(t, e.composer.Chat.Value, "second", "the recalled text must land in the input")

	// The run was never cancelled: releasing the server completes the first
	// turn, and the recalled prompt never reached the model.
	release()
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		s := e.transcript.Snapshot()
		// The final assistant event precedes loop exit; transcript completion
		// alone cannot prove that the submit gate has released the run.
		return len(s.Messages) >= 2 && !session.IsStreaming(s) && !ctrl.RunActive()
	})

	snap = e.transcript.Snapshot()
	require.Len(t, snap.Messages, 2, "user first, assistant first reply — nothing more")
	assert.Equal(t, session.RoleUser, snap.Messages[0].Role)
	assert.Equal(t, "first", snap.Messages[0].Text)
	assert.Equal(t, session.RoleAssistant, snap.Messages[1].Role)
	assert.Equal(t, "first reply", snap.Messages[1].FlatText())

	got := bodies()
	require.Len(t, got, 1, "the recalled prompt must never reach the model")
	assert.Contains(t, got[0], `"role":"user","content":"first"`)
	// The system prompt and tool schemas are a large blob; the precise
	// claim is that no second user message rode the request.
	assert.Equal(t, 1, strings.Count(got[0], `"role":"user"`),
		"the recalled prompt must not ride the request as a user message")

	// Queue empty, run over: the composer accepts a new submit again, with
	// the recalled text still in the input for editing.
	require.True(t, e.submitter.CanSubmit(), "input must be submittable again")
	assert.Equal(t, "second", e.composer.Chat.Value)
}
