package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestCalibratedTokensWithoutObservationIsEstimate(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)

	tokens, source := engine.calibratedTokens(12345)
	require.Equal(t, 12345, tokens, "with nothing to calibrate against the raw estimate rules")
	require.Equal(t, "estimate", source)
	require.Zero(t, engine.tokenOffset())
}

func TestCalibratedTokensAddsEstimatedDelta(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	// The last request estimated 10000 tokens and was billed 30000: 20000 of
	// system prompt, tool schemas and estimator error.
	engine.noteTokenObservation(10000, llm.Usage{PromptTokens: 30000})
	require.Equal(t, 20000, engine.tokenOffset())

	tokens, source := engine.calibratedTokens(12000)
	require.Equal(t, 32000, tokens, "the growth since the request rides on the provider's count")
	require.Equal(t, "calibrated", source)

	tokens, source = engine.calibratedTokens(10000)
	require.Equal(t, 30000, tokens, "nothing changed since the request: the count stands as reported")
	require.Equal(t, "provider", source)

	tokens, source = engine.calibratedTokens(5000)
	require.Equal(t, 25000, tokens, "a context that shrank counts down too")
	require.Equal(t, "calibrated", source)

	// A delta bigger than the whole reported count clamps at zero instead of
	// reporting a negative window.
	engine.noteTokenObservation(10000, llm.Usage{PromptTokens: 500})
	tokens, source = engine.calibratedTokens(1000)
	require.Zero(t, tokens)
	require.Equal(t, "calibrated", source)
}

func TestNoteTokenObservationIgnoresMissingUsage(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	engine.noteTokenObservation(10000, llm.Usage{PromptTokens: 30000})

	engine.noteTokenObservation(11000, llm.Usage{TotalTokens: 900})

	tokens, source := engine.calibratedTokens(10000)
	require.Equal(t, 30000, tokens, "a provider reporting no prompt tokens leaves the calibration alone")
	require.Equal(t, "provider", source)
}

func TestTokenObservationInvalidated(t *testing.T) {
	observed := func() *Engine {
		engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
		engine.noteTokenObservation(10000, llm.Usage{PromptTokens: 30000})
		_, source := engine.calibratedTokens(10000)
		require.Equal(t, "provider", source)
		return engine
	}

	compacted := observed()
	compacted.rearmCompactAdvice()
	_, source := compacted.calibratedTokens(10000)
	require.Equal(t, "estimate", source, "a compaction cut away the context the count was taken over")

	switched := observed()
	require.NoError(t, switched.SetModel(llm.ModelConfig{
		Name: "other", BaseURL: "http://127.0.0.1:1", APIKey: "x", ContextWindow: 100000,
	}))
	_, source = switched.calibratedTokens(10000)
	require.Equal(t, "estimate", source, "another model counts the same text differently")

	resumed := observed()
	require.NoError(t, resumed.ReplaceSession(SessionOpts{Cwd: t.TempDir()}))
	_, source = resumed.calibratedTokens(10000)
	require.Equal(t, "estimate", source, "a swapped session retires the old observation")
}

func TestContextStatsPrefersObservationOverLogUsage(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 1_000_000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "inspect the logs"},
		llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{bashCall("b1", "cat build.log")},
			Usage:     llm.Usage{PromptTokens: 50000, TotalTokens: 50100},
		},
	))

	// No in-memory observation yet — right after a resume, say: the usage
	// persisted in the session log is still the best answer.
	logged := engine.contextStats()
	require.Equal(t, "provider", logged.TokenSource)
	require.Equal(t, 50000, logged.ContextTokens)

	sent := estimateContextTokens(engine.inferenceContext(engine.sessionRef()))
	engine.noteTokenObservation(sent, llm.Usage{PromptTokens: 60000})
	// The tool result the round produced lands after that response: the log's
	// 50000 knows nothing about it, the calibration does.
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleTool, ToolCallID: "b1", Content: strings.Repeat("x", 40000)},
	))

	stats := engine.contextStats()
	grown := estimateContextTokens(engine.inferenceContext(engine.sessionRef())) - sent
	require.Positive(t, grown)
	require.Equal(t, "calibrated", stats.TokenSource)
	require.Equal(t, 60000+grown, stats.ContextTokens)
}

func TestProviderContextThresholdShiftsByOffset(t *testing.T) {
	// Window 100000 → trigger 83616, target 73616. The projection estimates
	// ~75000 tokens: under the trigger on its own, over it once the prompt
	// overhead the provider counts is taken into account.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "inspect the logs"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{bashCall("b1", "cat build.log")}},
		{Role: llm.RoleTool, ToolCallID: "b1", Content: strings.Repeat("x", 300000)},
		{Role: llm.RoleAssistant, Content: "noted"},
		{Role: llm.RoleUser, Content: "continue"},
	}
	require.NoError(t, engine.session.Append(history...))

	calm, calmReport := engine.providerContext(engine.sessionRef())
	require.Equal(t, 0, calmReport.Results, "the raw estimate stays under the trigger")
	require.Equal(t, history[2].Content, calm[2].Content)

	estimate := estimateContextTokens(calm)
	require.Less(t, estimate, 83616)
	// The same projection cost the provider 20000 tokens more than it
	// estimates: the real context is over the trigger, so the thresholds move
	// down by the overhead to meet the estimate where it lives.
	engine.noteTokenObservation(estimate, llm.Usage{PromptTokens: estimate + 20000})

	stubbed, stubReport := engine.providerContext(engine.sessionRef())
	require.Equal(t, 1, stubReport.Results, "the measured overhead pushes the projection past the trigger")
	require.Contains(t, stubbed[2].Content, "bash returned")
}

// The hard window guard reads the calibrated count, not the raw estimate: a
// context the provider already bills at 38000 of a 40000-token window has
// room for a few thousand more tokens, not for tens of thousands.
func TestWindowGuardUsesCalibratedEstimate(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseTextChunk())
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":38000,\"total_tokens\":38100}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	engine := newContextTestEngine(t, server.URL, 40000)

	var firstErr error
	for _, err := range engine.Loop(t.Context(), "hello", LoopOpts{}) {
		if err != nil {
			firstErr = err
		}
	}
	require.NoError(t, firstErr)
	require.Equal(t, int32(1), requests.Load())

	engine.mu.RLock()
	obs := engine.tokenObs
	engine.mu.RUnlock()
	require.NotNil(t, obs, "a response carrying usage calibrates the estimate")
	require.Equal(t, 38000, obs.prompt, "the provider's count for the projection that was sent")
	require.Positive(t, obs.estimate)

	// 40 KB of user text — ~10000 estimated tokens — on top of a context the
	// provider already counts at 38000. The raw estimate would see a couple
	// of kilobytes and send; the calibrated count sees the window overrun.
	require.NoError(t, engine.session.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: strings.Repeat("y", 40000),
	}))

	var nextErr error
	for _, err := range engine.Loop(t.Context(), "continue", LoopOpts{}) {
		if err != nil {
			nextErr = err
		}
	}
	require.ErrorIs(t, nextErr, ErrCompactionRequired)
	require.Equal(t, int32(1), requests.Load(), "the doomed request never leaves the engine")
}
