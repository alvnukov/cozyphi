package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// The executor's hard-compaction gate: past the hard strike count every tool
// but the context tool is refused before hooks, permissions or the handler
// itself spend anything — and the refusal carries the directive.
func TestExecutorCompactGateRefusesWorkBeforePermissions(t *testing.T) {
	var ran atomic.Int32
	gate := &recordingGate{}
	reg := tools.Registry{
		"bash": {
			Definition: llm.ToolDefinition{Name: "bash"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				ran.Add(1)
				return tools.Result{Content: "ok"}, nil
			},
		},
		"context": {
			Definition: llm.ToolDefinition{Name: "context"},
			Run: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: "compacted"}, nil
			},
		},
	}
	ex := NewExecutor(reg, gate, nil, nil)
	ex.SetCompactGate(func(tool string) string {
		if tool == "context" {
			return ""
		}
		return compactGateDirective()
	})

	msgs, _, _ := ex.run(t.Context(), []llm.ToolCall{
		{ID: "c1", Function: llm.Function{Name: "bash", Arguments: `{}`}},
		{ID: "c2", Function: llm.Function{Name: "context", Arguments: `{"action":"compact"}`}},
	}, func(session.ToolData) bool { return true })

	require.Equal(
		t,
		"context",
		gate.last.Tool,
		"only the allowed context call reaches permissions; bash is refused earlier",
	)
	assert.Equal(t, int32(0), ran.Load(), "the refused handler never runs")
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[0].Content, `{"action":"compact"}`, "the refusal tells the model how out")
	assert.Contains(t, msgs[1].Content, "compacted", "the context tool still runs")
}

// The full stop: the model ran a turn past the hard directive without
// compacting, so Loop refuses to run at all until a compaction lands.
func TestLoopRefusesToRunUnderCompactionStop(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	engine := newContextTestEngine(t, srv.URL, 30000)
	seedTwoTurnHistory(t, engine)

	for range compactStrikesStop {
		engine.noteCompactPressure()
	}
	require.True(t, engine.compactStopActive())

	var gotErr error
	for _, err := range engine.Loop(t.Context(), "hello", LoopOpts{}) {
		if err != nil {
			gotErr = err
		}
	}
	require.ErrorIs(t, gotErr, ErrCompactionRequired, "the stop surfaces as a distinguishable error")
	require.Zero(t, hits.Load(), "no inference request may leave a stopped engine")

	// A compaction (here: the rearm it triggers) releases the stop: the next
	// Loop reaches the provider instead of refusing at the door.
	engine.rearmCompactAdvice()
	require.False(t, engine.compactStopActive())
	sawError := false
	for _, err := range engine.Loop(t.Context(), "hello", LoopOpts{}) {
		if err != nil {
			sawError = true
			require.NotErrorIs(t, err, ErrCompactionRequired, "the stop is lifted")
		}
	}
	require.Positive(t, hits.Load(), "the released loop sends its inference request")
	require.True(t, sawError, "the fake 500 answers — proving real inference ran")
}
