package editor

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

func newQueueEditor(t *testing.T, baseURL string) (*Editor, *controller.Controller) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", baseURL)

	cwd := t.TempDir()
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

	e, ctrl := newQueueEditor(t, srv.URL)
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
