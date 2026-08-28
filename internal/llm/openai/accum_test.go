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
