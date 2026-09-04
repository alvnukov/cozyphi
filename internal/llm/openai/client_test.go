package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
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

func TestBuildRequestIncludesReasoningEffort(t *testing.T) {
	cfg := llm.ModelConfig{Name: "glm-5.2", ReasoningEffort: llm.ReasoningEffortHigh}
	req := BuildRequest(cfg, "", nil, nil)
	if req.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", req.ReasoningEffort)
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"reasoning_effort":"high"`)) {
		t.Fatalf("body = %s, want reasoning_effort field", body)
	}

	base := BuildRequest(llm.ModelConfig{Name: "glm-5.2"}, "", nil, nil)
	body, err = json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("reasoning_effort")) {
		t.Fatalf("body = %s, empty effort must omit reasoning_effort", body)
	}
}

// TestBuildRequestCarriesModelOptions: the model's configured options ride
// the chat-completions body under their openai wire names, and every unset
// field stays out of the JSON. The expected fragment mirrors opencode's
// documented config shape for a deepseek-style entry.
func TestBuildRequestCarriesModelOptions(t *testing.T) {
	temperature, topP := 1.0, 0.95
	cfg := llm.ModelConfig{Name: "deepseek-v4-pro[1m]", Options: llm.ModelOptions{
		Temperature:        &temperature,
		TopP:               &topP,
		ReasoningEffort:    "high",
		ChatTemplateKwargs: map[string]any{"thinking": true},
		EnableThinking:     new(false),
	}}
	body, err := json.Marshal(BuildRequest(cfg, "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"temperature":1`,
		`"top_p":0.95`,
		`"reasoning_effort":"high"`,
		`"chat_template_kwargs":{"thinking":true}`,
		`"enable_thinking":false`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("body = %s, want %s", body, want)
		}
	}

	// An empty options fragment sends none of the tuning fields: the wire
	// body must not carry a single one of them.
	bare, err := json.Marshal(BuildRequest(llm.ModelConfig{Name: "deepseek-v4-pro[1m]"}, "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"temperature", "top_p", "reasoning_effort", "chat_template_kwargs", "enable_thinking"} {
		if bytes.Contains(bare, []byte(unwanted)) {
			t.Fatalf("body = %s, unset %s must be omitted", bare, unwanted)
		}
	}
}

// TestBuildRequestVariantOverlaysModelOptions: selecting an effort that names
// a variant overlays the variant's fragment over the model's own options —
// the variant's effort wins, the model's temperature rides along, and the
// selected variant's raw thinking body reaches the wire.
func TestBuildRequestVariantOverlaysModelOptions(t *testing.T) {
	temperature := 0.6
	cfg := llm.ModelConfig{
		Name: "glm-5.2",
		Options: llm.ModelOptions{
			Temperature: &temperature, ReasoningEffort: "high",
			ChatTemplateKwargs: map[string]any{"thinking": true},
		},
		Variants: map[string]llm.VariantOptions{
			"max": {Options: llm.ModelOptions{ReasoningEffort: "max", Thinking: map[string]any{"type": "enabled"}}},
		},
		ReasoningEffort: llm.ReasoningEffortMax,
	}
	body, err := json.Marshal(BuildRequest(cfg, "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"temperature":0.6`,
		`"reasoning_effort":"max"`,
		`"thinking":{"type":"enabled"}`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("body = %s, want %s", body, want)
		}
	}
}
