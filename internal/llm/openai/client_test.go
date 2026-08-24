package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"
)

// TestStreamCapturesFinishReason: the provider's finish_reason rides the done
// event so the engine can tell a truncated round (length) from a clean end.
func TestStreamCapturesFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","content":"think"},"finish_reason":null}]}`+"\n\n",
		)
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"length"}]}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var done *llm.StreamEvent
	for ev, err := range StreamChatCompletion(
		t.Context(), server.Client(), server.URL, "k",
		BuildRequest(llm.ModelConfig{Name: "m"}, "", nil, nil),
	) {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if ev.Type == llm.StreamEventTypeDone {
			e := ev
			done = &e
		}
	}
	if done == nil {
		t.Fatal("expected done event")
	}
	if got := done.Partial.Choices[0].FinishReason; got != "length" {
		t.Fatalf("finish reason = %q, want length", got)
	}
}

// TestBuildRequestMaxTokensOptional: max_tokens is sent only when the model
// config sets an explicit output budget; providers apply their own default
// otherwise.
func TestBuildRequestMaxTokensOptional(t *testing.T) {
	cfg := llm.ModelConfig{Name: "deepseek-v4-pro", APIKey: "k", BaseURL: "https://api.example/v1"}
	req := BuildRequest(cfg, "", nil, nil)
	if req.MaxTokens != 0 {
		t.Fatalf("unset budget must not send max_tokens, got %d", req.MaxTokens)
	}
	cfg.MaxOutputTokens = 32768
	req = BuildRequest(cfg, "", nil, nil)
	if req.MaxTokens != 32768 {
		t.Fatalf("explicit budget = %d, want 32768", req.MaxTokens)
	}
}
