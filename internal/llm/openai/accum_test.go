package openai

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestStreamAccumulatorClampsHostileToolCallIndex pins the ordered rebuild
// against a stream-controlled index: the accumulator allocates by the highest
// index it accepted, so an out-of-bounds index must be dropped, not honored.
func TestStreamAccumulatorClampsHostileToolCallIndex(t *testing.T) {
	acc := newStreamAccumulator()
	acc.applyDelta(llm.StreamDelta{
		Content: "ok",
		ToolCalls: []llm.ToolCall{
			{Index: 1 << 30, ID: "hostile", Type: "function", Function: llm.Function{Name: "boom"}},
			{Index: -5, ID: "negative", Type: "function", Function: llm.Function{Name: "boom"}},
			{Index: 2, ID: "legit", Type: "function", Function: llm.Function{Name: "fine"}},
		},
	})

	msg := acc.message()
	if msg.Content != "ok" {
		t.Fatalf("content: %q", msg.Content)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "legit" {
		t.Fatalf("expected only the in-bounds call to survive, got %+v", msg.ToolCalls)
	}
}

// TestStreamAccumulatorAssemblesDeltas pins the happy-path contract: text and
// reasoning deltas concatenate in arrival order, a tool call's id/name/args
// fill in across separate deltas, and the final message reports the assistant
// role even when the stream never sent one.
func TestStreamAccumulatorAssemblesDeltas(t *testing.T) {
	acc := newStreamAccumulator()
	acc.applyDelta(llm.StreamDelta{ReasoningContent: "think "})
	acc.applyDelta(llm.StreamDelta{ReasoningContent: "hard"})
	acc.applyDelta(llm.StreamDelta{Content: "hel"})
	acc.applyDelta(llm.StreamDelta{Content: "lo"})
	acc.applyDelta(llm.StreamDelta{ToolCalls: []llm.ToolCall{
		{Index: 0, ID: "call-1", Type: "function", Function: llm.Function{Name: "read"}},
	}})
	acc.applyDelta(llm.StreamDelta{ToolCalls: []llm.ToolCall{
		{Index: 0, Function: llm.Function{Arguments: "{\"pa"}},
	}})
	acc.applyDelta(llm.StreamDelta{ToolCalls: []llm.ToolCall{
		{Index: 0, Function: llm.Function{Arguments: `th":"x"}`}},
	}})
	acc.applyDelta(llm.StreamDelta{Role: "assistant"})

	msg := acc.message()
	if msg.Role != llm.RoleAssistant {
		t.Fatalf("role: got %q, want assistant", msg.Role)
	}
	if msg.Content != "hello" {
		t.Fatalf("content: got %q, want hello", msg.Content)
	}
	if msg.ReasoningContent != "think hard" {
		t.Fatalf("reasoning: got %q, want 'think hard'", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls: got %d, want 1", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call-1" || tc.Type != "function" || tc.Function.Name != "read" {
		t.Fatalf("tool call identity: %+v", tc)
	}
	if tc.Function.Arguments != `{"path":"x"}` {
		t.Fatalf("arguments: got %q", tc.Function.Arguments)
	}
}

// TestStreamAccumulatorDefaultsRole pins the assistant default: streams that
// never send a role delta still produce a usable message.
func TestStreamAccumulatorDefaultsRole(t *testing.T) {
	acc := newStreamAccumulator()
	acc.applyDelta(llm.StreamDelta{Content: "hi"})

	if got := acc.message().Role; got != llm.RoleAssistant {
		t.Fatalf("role default: got %q, want assistant", got)
	}
}

// TestStreamAccumulatorApplyMessageReplacesText pins the final-delta
// override: a stream that ends with a fully-formed message replaces the
// accumulated text instead of appending to it.
func TestStreamAccumulatorApplyMessageReplacesText(t *testing.T) {
	acc := newStreamAccumulator()
	acc.applyDelta(llm.StreamDelta{Content: "partial "})
	acc.applyMessage(&llm.Message{Content: "final", ReasoningContent: "done"})

	msg := acc.message()
	if msg.Content != "final" {
		t.Fatalf("content: got %q, want final", msg.Content)
	}
	if msg.ReasoningContent != "done" {
		t.Fatalf("reasoning: got %q, want done", msg.ReasoningContent)
	}
}
