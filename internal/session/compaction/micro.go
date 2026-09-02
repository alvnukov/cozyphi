package compaction

import (
	"slices"
	"strconv"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// microFloorBytes is the smallest tool result worth stubbing. Below it the
// stub saves almost nothing and costs fidelity.
const microFloorBytes = 2048

// MicroReport summarizes one microcompaction pass.
type MicroReport struct {
	// Results is the number of tool results replaced with stubs.
	Results int
	// BytesElided is the total content bytes removed by those stubs.
	BytesElided int
}

// Microcompact returns a provider-view projection of messages in which tool
// results older than the keep-recent tail are replaced with short metadata
// stubs. The projection is lossless for the session log — the caller sends
// the result to the provider instead of the raw history — and reversible for
// the model, because each stub names its tool and says how to recover the
// output.
//
// A tool result is left verbatim when any of these holds:
//   - it falls inside the keep-recent tail (the same keepRecentTokens budget
//     the macro cut uses, so both layers agree on what counts as recent);
//   - keepVerbatim(tool, args) reports true — anchor-carrying results such as
//     editable reads and greps must survive intact or hashline edits break;
//   - its content is below microFloorBytes;
//   - its tool call cannot be resolved (fail closed: keep everything).
//
// keepVerbatim nil disables the pass entirely. The projection is idempotent:
// a stub is far below the floor, so a second pass changes nothing.
func Microcompact(
	messages []llm.Message,
	s Settings,
	keepVerbatim func(tool, args string) bool,
) ([]llm.Message, MicroReport) {
	if keepVerbatim == nil || len(messages) == 0 {
		return messages, MicroReport{}
	}

	cutoff := microCutoff(messages, s.keepRecentTokens)
	if cutoff <= 0 {
		return messages, MicroReport{}
	}
	calls := toolCallsByID(messages)

	projected := make([]llm.Message, len(messages))
	copy(projected, messages)
	var report MicroReport
	for i := range cutoff {
		msg := projected[i]
		if msg.Role != llm.RoleTool {
			continue
		}
		call, ok := calls[msg.ToolCallID]
		if !ok || keepVerbatim(call.Function.Name, call.Function.Arguments) {
			continue
		}
		if len(msg.Content) < microFloorBytes {
			continue
		}
		stubbed := msg
		stubbed.Content = microStub(call.Function.Name, msg.Content)
		projected[i] = stubbed
		report.Results++
		report.BytesElided += len(msg.Content) - len(stubbed.Content)
	}
	return projected, report
}

// microCutoff returns the number of leading messages that fall outside the
// keep-recent tail: the index after the oldest message whose removal still
// leaves the remaining suffix within keepRecentTokens estimated tokens.
func microCutoff(messages []llm.Message, keepRecentTokens int) int {
	budget := keepRecentTokens
	for i, msg := range slices.Backward(messages) {
		cost := estimateMessageTokens(msg)
		if cost > budget {
			return i + 1
		}
		budget -= cost
	}
	return 0
}

// toolCallsByID indexes assistant tool calls by ID so a RoleTool message can
// be resolved to its tool name and arguments. RepairToolHistory has already
// paired every surviving result with its call by the time a projection runs.
func toolCallsByID(messages []llm.Message) map[string]llm.ToolCall {
	var count int
	for _, msg := range messages {
		if msg.Role == llm.RoleAssistant {
			count += len(msg.ToolCalls)
		}
	}
	if count == 0 {
		return nil
	}
	calls := make(map[string]llm.ToolCall, count)
	for _, msg := range messages {
		if msg.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			calls[call.ID] = call
		}
	}
	return calls
}

// microStub renders the replacement content for an elided tool result. It
// keeps the tool identity and the output's shape so the model can decide
// whether re-running the tool is worth a round trip.
func microStub(tool, content string) string {
	lines := 0
	if content != "" {
		lines = strings.Count(content, "\n") + 1
	}
	var b strings.Builder
	b.WriteString("[tool result elided by microcompaction: ")
	b.WriteString(tool)
	b.WriteString(" returned ")
	writeCount(&b, lines, "line")
	b.WriteString(" (")
	writeCount(&b, len(content), "byte")
	b.WriteString("); re-run ")
	b.WriteString(tool)
	b.WriteString(" to recover the output]")
	return b.String()
}

func writeCount(b *strings.Builder, n int, unit string) {
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" ")
	b.WriteString(unit)
	if n != 1 {
		b.WriteString("s")
	}
}
