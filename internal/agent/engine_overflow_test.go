package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

// seedLargeHistory appends turns of padded messages so the auto-compaction
// cut has older history to summarize.
func seedLargeHistory(t *testing.T, engine *Engine, turns int) {
	t.Helper()
	pad := strings.Repeat("x", 2000)
	for i := range turns {
		require.NoError(t, engine.session.Append(
			llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("SEED-U-%d %s", i, pad)},
			llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("SEED-A-%d %s", i, pad)},
		))
	}
}

// overflowContextServer answers the first streaming request with a
// context-overflow rejection, any non-streaming request with a summary, and
// subsequent streaming requests with a normal assistant text chunk.
func overflowContextServer(t *testing.T, summaryText string) (*httptest.Server, *atomic.Int32, func() []string) {
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

		if streams.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(
				w,
				`{"error":{"message":"This model's maximum context length is 100000 tokens.","code":"context_length_exceeded"}}`,
			)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, &streams, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func TestLoopRetriesAfterContextOverflow(t *testing.T) {
	server, streams, bodies := overflowContextServer(t, "SUMMARY-OF-OVERFLOW-HISTORY")
	engine := newContextTestEngine(t, server.URL, 100000)
	seedLargeHistory(t, engine, 40)

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
	require.True(t, started, "overflow must trigger compaction")
	require.True(t, complete, "overflow compaction must complete")
	require.False(t, failed)
	require.Equal(t, int32(2), streams.Load(), "one rejected stream + one retry stream")

	snapshot := bodies()
	require.Len(t, snapshot, 3, "stream, compaction, retry stream")

	retry := snapshot[2]
	require.Contains(t, retry, "SUMMARY-OF-OVERFLOW-HISTORY")
	require.NotContains(t, retry, "SEED-U-0", "summarized history must leave the retry context")
}

func TestLoopNonOverflowErrorDoesNotCompact(t *testing.T) {
	var streams atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		streams.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"invalid_request_error"}}`)
	}))
	t.Cleanup(server.Close)

	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	var (
		compacted bool
		lastErr   error
	)
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		if err != nil {
			lastErr = err
			break
		}
		if _, ok := ev.(session.CompactionStarted); ok {
			compacted = true
		}
	}
	require.ErrorContains(t, lastErr, "invalid_request_error")
	require.False(t, compacted, "non-overflow errors must fail fast without compaction")
	require.Equal(t, int32(1), streams.Load(), "no retry for non-overflow errors")
}
