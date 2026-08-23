package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func newContextTestEngine(t *testing.T, url string, window int) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: url, APIKey: "x", ContextWindow: window},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	return engine
}

// seedTwoTurnHistory leaves two turns of history where the last assistant
// message reports enough tokens to push the compaction cut into turn one.
func seedTwoTurnHistory(t *testing.T, engine *Engine) {
	t.Helper()
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "SEED-A-SENTINEL first turn question"},
		llm.Message{Role: llm.RoleAssistant, Content: "first turn answer"},
		llm.Message{Role: llm.RoleUser, Content: "SEED-B-SENTINEL second turn question"},
		llm.Message{Role: llm.RoleAssistant, Content: "second turn answer", Usage: llm.Usage{TotalTokens: 25000}},
	))
}

func TestEngineContextStatsReportsProviderUsage(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "SEED-A-SENTINEL"},
		llm.Message{Role: llm.RoleAssistant, Content: "ok", Usage: llm.Usage{PromptTokens: 4242, TotalTokens: 4300}},
	))

	stats := engine.contextStats()
	require.Equal(t, 4242, stats.ContextTokens)
	require.Equal(t, "provider", stats.TokenSource)
	require.Equal(t, 2, stats.Messages)
	require.Positive(t, stats.UsedBytes)
	require.Equal(t, 100000, stats.ContextWindow)
	require.Equal(t, 100000-16384, stats.ThresholdTokens)
	require.False(t, stats.CompactionRecommended)
}

func TestEngineContextStatsEstimatesWithoutUsage(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, engine.session.Append(llm.Message{Role: llm.RoleUser, Content: "hello"}))

	stats := engine.contextStats()
	require.Equal(t, "estimate", stats.TokenSource)
	require.Equal(t, stats.UsedBytes/4, stats.ContextTokens)
	require.False(t, stats.CompactionRecommended)
}

func TestEngineContextStatsUnknownWindow(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 0)
	stats := engine.contextStats()
	require.Zero(t, stats.ContextWindow)
	require.Zero(t, stats.ThresholdTokens)
	require.False(t, stats.CompactionRecommended)
}

func TestEngineRequestCompactGuards(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.ErrorContains(t, engine.requestCompact(), "nothing to compact")

	seedTwoTurnHistory(t, engine)
	require.NoError(t, engine.requestCompact())
	require.True(t, engine.pendingCompact)
	require.ErrorContains(t, engine.requestCompact(), "already scheduled")
}

// fakeContextServer records request bodies and separates streaming chat
// requests (answered with SSE) from non-streaming compaction requests
// (answered with a JSON completion carrying summaryText).
func fakeContextServer(
	t *testing.T,
	summaryText string,
	sseResponse func(streamNumber int32) string,
) (*httptest.Server, *atomic.Int32, func() []string) {
	t.Helper()
	var (
		mu      sync.Mutex
		bodies  []string
		streams atomic.Int32
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()

		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(raw, &req)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, summaryText)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseResponse(streams.Add(1)))
		// A usage-bearing chunk with no choices, the OpenAI way.
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":26000,\"total_tokens\":26500}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, &streams, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func TestLoopContextToolStatusReachesModel(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused summary", func(n int32) string {
		if n == 1 {
			return sseToolCallChunk("call_1", "context", `{}`)
		}
		return sseTextChunk()
	})

	engine := newContextTestEngine(t, server.URL, 100000)

	var contextToolDone bool
	var lastErr error
	for ev, err := range engine.Loop(t.Context(), "TOPSECRET-PROMPT-SENTINEL", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		if td, ok := ev.(session.ToolData); ok && td.Run.Name == "context" && td.Run.Status == session.ToolDone {
			contextToolDone = true
		}
	}
	require.NoError(t, lastErr)
	require.True(t, contextToolDone, "context tool call must execute and complete")
	require.Equal(t, int32(2), streams.Load())

	snapshot := bodies()
	require.Len(t, snapshot, 2)
	// The status report reaches the model as the tool result of round two.
	// (Inside the request body the tool-result JSON quotes are escaped.)
	// The tool-calling assistant carried provider usage, so the report cites it.
	require.Contains(t, snapshot[1], "context_tokens")
	require.Contains(t, snapshot[1], `token_source\":\"provider\"`)
	require.Contains(t, snapshot[1], "context_kb")
}

func TestLoopContextToolCompactRunsAtRoundBoundary(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(n int32) string {
		if n == 1 {
			return sseToolCallChunk("call_1", "context", `{"action":"compact"}`)
		}
		return sseTextChunk()
	})

	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	var (
		started  bool
		complete bool
		failed   bool
		lastErr  error
	)
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		switch e := ev.(type) {
		case session.CompactionStarted:
			started = true
		case session.CompactionComplete:
			complete = true
			failed = e.Failed
		}
	}
	require.NoError(t, lastErr)
	require.True(t, started, "boundary compaction must signal CompactionStarted")
	require.True(t, complete, "boundary compaction must signal CompactionComplete")
	require.False(t, failed)
	require.False(t, engine.pendingCompact, "request is consumed when applied")
	require.Equal(t, int32(2), streams.Load())

	snapshot := bodies()
	// stream 1 + history summary + split-turn prefix summary + stream 2
	require.Len(t, snapshot, 4)

	// The final "done" assistant lands after the compaction entry, so scan
	// for the entry instead of asserting on the last one.
	var compEntry *session.CompactionEntry
	for _, entry := range engine.session.PathEntries() {
		if entry.GetType() == session.EntryCompaction {
			c := entry.(session.CompactionEntry)
			compEntry = &c
		}
	}
	require.NotNil(t, compEntry, "boundary compaction must append a compaction entry")
	require.Contains(t, compEntry.Compaction.Summary, "SUMMARY-OF-OLD-HISTORY")
	// TokensBefore is the usage reported by the newest assistant at
	// compaction time — the tool-calling round, not the seeded history.
	require.Equal(t, 26500, compEntry.Compaction.TokensBefore)

	// Round two sees the summary instead of turn one, and the current
	// assistant tool-call + result pair survives verbatim.
	secondStream := snapshot[len(snapshot)-1]
	require.Contains(t, secondStream, "SUMMARY-OF-OLD-HISTORY")
	require.Contains(t, secondStream, "scheduled")
	require.Contains(t, secondStream, "call_1")
	require.NotContains(t, secondStream, "SEED-A-SENTINEL", "summarized history must leave the model view")
}
