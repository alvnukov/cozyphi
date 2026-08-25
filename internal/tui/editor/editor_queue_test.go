package editor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func newQueueEditor(t *testing.T, baseURL, cwd string) (*Editor, *controller.Controller) {
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

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 1000, nil, nil)
	return e, ctrl
}

// submitPrompt drives a full user submit through the editor's key handling:
// text lands in the composer, Enter reaches ChatInput, and the submitter
// appends the user row and hands the prompt to the controller.
func submitPrompt(e *Editor, text string) {
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
// queue: a prompt submitted while the model is answering is shown as queued,
// the marker clears when the in-flight turn finishes, and the queued prompt is
// then sent to the model and answered — all through Editor.Handle, the bus, and
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

	// Submit while the model is still answering: the row must be marked queued.
	submitPrompt(e, "second")
	e.DrainNow()

	snap := e.transcript.Snapshot()
	var second *session.Message
	for i := range snap.Messages {
		if snap.Messages[i].Role == session.RoleUser && snap.Messages[i].Text == "second" {
			second = &snap.Messages[i]
		}
	}
	require.NotNil(t, second, "second user row must be in the transcript")
	require.True(t, second.Queued, "second user row must be marked (queued) while the first turn runs")

	// Finish the first turn: the controller must dequeue the second prompt and
	// answer it, clearing the queued marker on the way.
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
	assert.False(t, snap.Messages[2].Queued, "queued marker must clear after dequeue")
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

// TestEditorQueuedRowStaysInPlaceUntilDelivered pins the transcript semantics
// the user asked for: a row submitted mid-run stays exactly where it was
// submitted, and nothing may render below it while it is still queued. The
// (queued) hint clears at the moment the model receives the message — the
// tool-round boundary — which is visible in the final order: round 1, the
// user row, then the model's answer to it.
func TestEditorQueuedRowStaysInPlaceUntilDelivered(t *testing.T) {
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

	// Submit while round 1 is mid-stream: the row lands below the streaming
	// assistant, marked queued, with nothing below it yet.
	submitPrompt(e, "hold my place")
	e.DrainNow()
	snap := e.transcript.Snapshot()
	require.Len(t, snap.Messages, 3, "user, streaming assistant, queued user")
	queued := snap.Messages[2]
	require.Equal(t, session.RoleUser, queued.Role)
	require.Equal(t, "hold my place", queued.Text)
	require.True(t, queued.Queued, "row must be marked (queued) while undelivered")
	queuedID := queued.ID

	// Release round 1: the tool runs, the boundary delivers the queued row
	// mid-turn, and round 2 answers it below. Every sampled snapshot must keep
	// the row at its index, and any content below it requires delivery first.
	release()
	waitFor(t, 10*time.Second, func() bool {
		e.DrainNow()
		s := e.transcript.Snapshot()
		require.Equal(t, 1, countUserRows(s, "hold my place"),
			"the queued row must not be duplicated by the mid-turn injection")
		if s.Messages[2].ID != queuedID || s.Messages[2].Text != "hold my place" {
			t.Fatalf("queued row moved: got %+v at index 2", s.Messages[2])
		}
		if len(s.Messages) > 3 && s.Messages[2].Queued {
			t.Fatal("content rendered below the row while it was still queued")
		}
		return len(s.Messages) >= 4 && !session.IsStreaming(s)
	})

	snap = e.transcript.Snapshot()
	require.Len(t, snap.Messages, 4, "round 1, queued row, round 2")
	assert.Equal(t, session.RoleAssistant, snap.Messages[1].Role)
	assert.Equal(t, "round one ", snap.Messages[1].FlatText())
	assert.Equal(t, queuedID, snap.Messages[2].ID, "same row, same place")
	assert.False(t, snap.Messages[2].Queued, "hint clears when the model sees the row")
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
