package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
	llmclient "github.com/alvnukov/cozyphi/internal/llm/client"
	"github.com/alvnukov/cozyphi/internal/session"
)

// newAnthropicRoundRT builds a round runtime whose client speaks the
// Anthropic wire protocol against ts.
func newAnthropicRoundRT(t *testing.T, handler http.HandlerFunc) roundRuntime {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	cfg := llm.ModelConfig{
		Name:       "claude-test",
		ProviderID: "anthropic",
		Protocol:   llm.ProtocolAnthropic,
		BaseURL:    ts.URL,
		APIKey:     "test-key",
	}
	rt := roundRuntime{modelName: cfg.Name}
	rt.client = llmclient.NewClient(cfg, nil, "system")
	return rt
}

// TestStreamTurnPreservesStatusForClassification: a provider HTTP rejection
// (here 429) must reach the engine's caller as a typed, wrappable error, so
// the TUI and `phi run` branch on the status instead of grepping text.
func TestStreamTurnPreservesStatusForClassification(t *testing.T) {
	rt := newAnthropicRoundRT(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	})

	_, _, _, err := streamTurn(t.Context(),
		func(session.Event, error) bool { return true },
		nil, rt)

	if err == nil {
		t.Fatal("expected the 429 to fail the round")
	}
	if !llm.IsRateLimited(err) {
		t.Fatalf("rate-limit signal lost through the engine: %v", err)
	}
}

// TestStreamTurnPreservesAuthStatus: a 401 keeps the auth signal so the run
// error can point at /connect instead of a generic failure.
func TestStreamTurnPreservesAuthStatus(t *testing.T) {
	rt := newAnthropicRoundRT(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
	})

	_, _, _, err := streamTurn(t.Context(),
		func(session.Event, error) bool { return true },
		nil, rt)

	if err == nil {
		t.Fatal("expected the 401 to fail the round")
	}
	if !llm.IsAuthFailure(err) {
		t.Fatalf("auth signal lost through the engine: %v", err)
	}
}
