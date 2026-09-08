package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestStreamAcceptsReasoningAlias: an OpenAI-compatible server that spells the
// thinking field "reasoning" — Ollama's compatibility layer tags it that way on
// both its Message and its streaming Delta — must reach the engine as reasoning
// content. Otherwise a local reasoning model streams nothing but empty content
// deltas and its whole turn is dropped. The fallback order follows
// @ai-sdk/openai-compatible: reasoning_content when present, reasoning
// otherwise.
func TestStreamAcceptsReasoningAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","content":"","reasoning":"weighing "}}]}`+"\n\n",
		)
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"","reasoning":"options"}}]}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var streamed string
	var done *llm.StreamEvent
	for ev, err := range StreamChatCompletion(
		t.Context(), server.Client(), server.URL, "k",
		BuildRequest(llm.ModelConfig{Name: "m"}, "", nil, nil),
	) {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if ev.Type == llm.StreamEventTypeDelta {
			streamed += ev.Delta.ReasoningContent
		}
		if ev.Type == llm.StreamEventTypeDone {
			e := ev
			done = &e
		}
	}
	if streamed != "weighing options" {
		t.Fatalf("streamed reasoning = %q, want %q", streamed, "weighing options")
	}
	if done == nil {
		t.Fatal("expected done event")
	}
	msg := done.Partial.Choices[0].Message
	if msg.ReasoningContent != "weighing options" {
		t.Fatalf("message reasoning = %q, want %q", msg.ReasoningContent, "weighing options")
	}
	if msg.Content != "done" {
		t.Fatalf("message content = %q, want %q", msg.Content, "done")
	}
}

// TestStreamAcceptsReasoningAliasOnWholeMessage: some OpenAI-compatible
// servers put the finished message in the chunk instead of deltas. The alias
// has to be read there too, or the same thinking is lost on that path.
func TestStreamAcceptsReasoningAliasOnWholeMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{},"message":{"role":"assistant","content":"done","reasoning":"weighed it"},"finish_reason":"stop"}]}`+"\n\n",
		)
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
	if got := done.Partial.Choices[0].Message.ReasoningContent; got != "weighed it" {
		t.Fatalf("message reasoning = %q, want %q", got, "weighed it")
	}
}

// TestStreamPrefersCanonicalReasoningField: when a server sends both
// spellings, reasoning_content wins — the alias is a fallback, not an
// override, matching @ai-sdk/openai-compatible's `reasoning_content ??
// reasoning`.
func TestStreamPrefersCanonicalReasoningField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			`data: {"choices":[{"delta":{"role":"assistant","reasoning_content":"canonical","reasoning":"alias"}}]}`+"\n\n",
		)
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
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
	if got := done.Partial.Choices[0].Message.ReasoningContent; got != "canonical" {
		t.Fatalf("message reasoning = %q, want canonical", got)
	}
}
