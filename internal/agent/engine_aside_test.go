package agent

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// asideServer answers every request with the script it was given and keeps
// the request bodies, so a test can read what the model was sent.
type asideServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newAsideServer(t *testing.T, script func(w http.ResponseWriter, r *http.Request)) *asideServer {
	t.Helper()
	s := &asideServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(body))
		s.mu.Unlock()
		script(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *asideServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func answerWith(chunks ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			_, _ = fmt.Fprint(w, chunk)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func newAsideTestEngine(t *testing.T, serverURL string, runs *atomic.Int32) *Engine {
	t.Helper()
	dir := t.TempDir()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: serverURL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true, Model: "fake"},
		Gate:        permission.AllowAll{},
		Tools:       []tools.Tool{countingTool(runs)},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.session.Close() })
	return engine
}

func collectAside(events *[]session.AsideUpdate) func(session.Event) bool {
	return func(ev session.Event) bool {
		if update, ok := ev.(session.AsideUpdate); ok {
			*events = append(*events, update)
		}
		return true
	}
}

// A side question is answered from the context as it stands and leaves every
// piece of it where it was: the cursor, the context, the cache of it and the
// stub set a turn keeps between rounds. The answer is written to the file
// under the id it streamed under, and a reload still keeps it out of the
// context.
func TestEngineAsideAnswersFromTheContextAndLeavesItAlone(t *testing.T) {
	server := newAsideServer(t, answerWith(sseTextChunk()))
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	_, answer := seedTwoTurns(t, engine)
	contextBefore := engine.session.BuildContext()
	require.True(t, engine.session.contextCacheValid, "the cache is warm before the question")
	engine.microStubbed = map[string]struct{}{"frozen": {}}

	var events []session.AsideUpdate
	require.NoError(t, engine.Aside(t.Context(), "  what did we do?  ", "", collectAside(&events)))

	require.NotEmpty(t, events)
	id := events[0].ID
	require.NotEmpty(t, id)
	for _, update := range events {
		assert.Equal(t, id, update.ID, "the row keeps one id from the first token to the last")
	}
	last := events[len(events)-1]
	assert.Equal(t, session.StateComplete, last.State)
	assert.Equal(t, "done", last.Answer)
	assert.Equal(t, answer, last.Anchor, "an empty anchor resolves to the end of the context")

	requests := server.requests()
	require.Len(t, requests, 1)
	for _, want := range []string{"first question", "second answer", "what did we do?", "Do not call any tools"} {
		assert.Contains(t, requests[0], want)
	}

	assert.Equal(t, answer, engine.session.manager.LeafID(), "the cursor did not move")
	assert.True(t, engine.session.contextCacheValid, "the cache was neither dropped nor rebuilt")
	assert.Equal(t, contextBefore, engine.session.BuildContext())
	assert.Equal(t, map[string]struct{}{"frozen": {}}, engine.microStubbed)

	asides := engine.session.manager.Asides()
	require.Len(t, asides, 1)
	assert.Equal(t, id, asides[0].ID)
	assert.Equal(t, "what did we do?", asides[0].Question)
	assert.Equal(t, "done", asides[0].Answer)
	assert.Equal(t, answer, asides[0].Leaf)
	assert.Equal(t, "fake", asides[0].Model)

	path := engine.session.manager.File()
	require.NoError(t, engine.session.Close())
	reopened, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	assert.Len(t, reopened.BuildContext(), 4, "a reload keeps the aside out of the context")
	assert.Len(t, reopened.Asides(), 1)
}

// A named anchor answers from the context as it stood at that message: what
// came after it is not sent.
func TestEngineAsideAtAnAnchorSendsTheContextUpToIt(t *testing.T) {
	server := newAsideServer(t, answerWith(sseTextChunk()))
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	seedTwoTurns(t, engine)
	firstAnswer := engine.ContextReport().Items[1].EntryID

	var events []session.AsideUpdate
	require.NoError(t, engine.Aside(t.Context(), "why?", firstAnswer, collectAside(&events)))

	requests := server.requests()
	require.Len(t, requests, 1)
	assert.Contains(t, requests[0], "first answer")
	assert.NotContains(t, requests[0], "second question")
	assert.Equal(t, firstAnswer, engine.session.manager.Asides()[0].Anchor)
}

// Tools never run for a side question. A call the model makes anyway ends
// the answer, nothing is executed, no second round is sent, and the record
// says which tool was asked for.
func TestEngineAsideNeverRunsAToolTheModelCalls(t *testing.T) {
	server := newAsideServer(t, answerWith(sseToolCallChunk("call_1", "count", `{}`)))
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	_, answer := seedTwoTurns(t, engine)

	var events []session.AsideUpdate
	require.NoError(t, engine.Aside(t.Context(), "count for me", "", collectAside(&events)))

	assert.Zero(t, runs.Load(), "the tool the model called never ran")
	assert.Len(t, server.requests(), 1, "no round follows the call")
	last := events[len(events)-1]
	assert.Equal(t, session.StateComplete, last.State)
	assert.Equal(t, "count", last.SkippedTool)
	asides := engine.session.manager.Asides()
	require.Len(t, asides, 1)
	assert.Equal(t, "count", asides[0].SkippedTool)
	assert.Equal(t, answer, engine.session.manager.LeafID())
	assert.Len(t, engine.session.BuildContext(), 4, "neither the call nor a result joined the context")
}

// Everything that can refuse a question does so before the model is asked
// and before anything is written.
func TestEngineAsideRefusesBeforeAskingTheModel(t *testing.T) {
	server := newAsideServer(t, answerWith(sseTextChunk()))
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)

	err := engine.Aside(t.Context(), "anything?", "", collectAside(new([]session.AsideUpdate)))
	require.ErrorIs(t, err, session.ErrNothingToAsk, "an empty session has nothing to ask about")

	seedTwoTurns(t, engine)
	err = engine.Aside(t.Context(), "   ", "", collectAside(new([]session.AsideUpdate)))
	require.ErrorIs(t, err, session.ErrEmptyAsideQuestion)

	err = engine.Aside(t.Context(), "why?", "nothing-like-this", collectAside(new([]session.AsideUpdate)))
	var notAnchor *session.NotAsideAnchorError
	require.ErrorAs(t, err, &notAnchor)

	assert.Empty(t, server.requests(), "no refusal reached the model")
	assert.Empty(t, engine.session.manager.Asides(), "no refusal reached the file")
}

// Asking on the side while a turn is running is refused: the context it
// would be asked about is still being written.
func TestEngineRefusesAnAsideDuringATurn(t *testing.T) {
	var (
		mu       sync.Mutex
		engine   *Engine
		asideErr error
		asked    bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		if !asked {
			asked = true
			_, asideErr = engine.PrepareAside("side?", "")
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	var runs atomic.Int32
	engine = newAsideTestEngine(t, server.URL, &runs)
	seedTwoTurns(t, engine)

	for range engine.Loop(t.Context(), "third question", LoopOpts{}) { //nolint:revive // the turn is what matters
	}

	mu.Lock()
	defer mu.Unlock()
	require.True(t, asked, "the handler ran, so the refusal was measured inside a turn")
	require.ErrorIs(t, asideErr, ErrAsideWhileTurnRunning)
	assert.Empty(t, engine.session.manager.Asides())
}

// A question cut short is not written down: the file keeps only answers
// that were given in full, and the row says the answer was cancelled.
func TestEngineAsideCancelledMidAnswerLeavesNoRecord(t *testing.T) {
	server := newAsideServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	_, answer := seedTwoTurns(t, engine)
	contextBefore := engine.session.BuildContext()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var events []session.AsideUpdate
	err := engine.Aside(ctx, "side?", "", func(ev session.Event) bool {
		update, ok := ev.(session.AsideUpdate)
		if ok {
			events = append(events, update)
			if update.State == session.StateStreaming {
				cancel()
			}
		}
		return true
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NotEmpty(t, events)
	assert.Equal(t, session.StateCancelled, events[len(events)-1].State)
	assert.Empty(t, engine.session.manager.Asides(), "a cancelled answer is not recorded")
	assert.Equal(t, answer, engine.session.manager.LeafID())
	assert.Equal(t, contextBefore, engine.session.BuildContext())
}

// A provider failure ends the row in an error that says why, and nothing is
// written.
func TestEngineAsideFailureIsShownAndNotRecorded(t *testing.T) {
	server := newAsideServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	})
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	seedTwoTurns(t, engine)

	var events []session.AsideUpdate
	err := engine.Aside(t.Context(), "side?", "", collectAside(&events))
	require.Error(t, err)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, session.StateError, last.State)
	assert.NotEmpty(t, strings.TrimSpace(last.Error), "the row says what went wrong")
	assert.Empty(t, engine.session.manager.Asides())
}

// The answer stops at the first tool call as it streams in, rather than
// waiting for the rest of the reply: whatever the model says after asking
// for a tool is not part of the answer.
func TestEngineAsideStopsAtTheFirstToolCall(t *testing.T) {
	server := newAsideServer(t, answerWith(
		"data: "+sseDelta(`{"role":"assistant","content":"let me check"}`)+"\n\n",
		sseToolCallChunk("call_1", "count", `{}`),
		"data: "+sseDelta(`{"content":" and more after the call"}`)+"\n\n",
	))
	var runs atomic.Int32
	engine := newAsideTestEngine(t, server.URL, &runs)
	seedTwoTurns(t, engine)

	var events []session.AsideUpdate
	require.NoError(t, engine.Aside(t.Context(), "count for me", "", collectAside(&events)))

	last := events[len(events)-1]
	assert.Equal(t, "let me check", last.Answer)
	assert.Equal(t, "count", last.SkippedTool)
	assert.Equal(t, "let me check", engine.session.manager.Asides()[0].Answer)
	assert.Zero(t, runs.Load())
}

func sseDelta(delta string) string {
	return `{"choices":[{"delta":` + delta + `}]}`
}

// Esc can land after the provider has finished but before the answer is
// written. The row is cancelled by then, so the file must not keep the answer
// either.
func TestEngineAsideCancelledAfterTheLastTokenLeavesNoRecord(t *testing.T) {
	var runs atomic.Int32
	engine := newAsideTestEngine(t, "http://127.0.0.1:1", &runs)
	_, answer := seedTwoTurns(t, engine)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	engine.asideStream = func(context.Context, []llm.Message) iter.Seq2[llm.StreamEvent, error] {
		return func(yield func(llm.StreamEvent, error) bool) {
			done := llm.StreamEvent{Type: llm.StreamEventTypeDone, Partial: llm.Response{
				Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: "all of it"}}},
			}}
			if !yield(done, nil) {
				cancel()
				return
			}
			cancel()
		}
	}

	var events []session.AsideUpdate
	err := engine.Aside(ctx, "side?", "", collectAside(&events))
	require.ErrorIs(t, err, context.Canceled)

	require.NotEmpty(t, events)
	assert.Equal(t, session.StateCancelled, events[len(events)-1].State)
	assert.Empty(t, engine.session.manager.Asides(), "the answer the user cancelled is not recorded")
	assert.Equal(t, answer, engine.session.manager.LeafID())
	raw, err := os.ReadFile(engine.session.manager.File())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"type":"aside"`)
}
