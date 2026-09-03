package compaction

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// microFloorBytes is the smallest tool result worth stubbing. Below it the
// stub saves almost nothing and costs fidelity. It also makes the projection
// idempotent: every stub lands far below the floor, so a second pass over an
// already-projected history changes nothing.
const microFloorBytes = 2048

// microFirstLineBytes bounds the shred of the original output a stub keeps.
const microFirstLineBytes = 200

// MicroReport summarizes one microcompaction pass.
type MicroReport struct {
	// Results is the number of tool results replaced with stubs.
	Results int
	// BytesElided is the total content bytes removed by those stubs.
	BytesElided int
}

// MicroPolicy shapes one provider-view microcompaction pass.
type MicroPolicy struct {
	// KeepVerbatim reports tool results that must never be stubbed: anchor
	// carriers whose LINE#HASH lines authorize edits, answers the user typed,
	// reports that no re-run can reproduce. nil makes nothing a candidate —
	// a frozen set is still applied, so the projection stays stable.
	KeepVerbatim func(tool, args string) bool
	// Advice returns the recovery sentence a stub carries for its tool; ""
	// (or a nil Advice) falls back to the generic "Re-run <tool> with the
	// same arguments to recover the output."
	Advice func(tool, args string) string
	// KeepRecentTokens is the verbatim tail budget — the macro cut's
	// keep-recent, so both layers agree on what counts as recent.
	KeepRecentTokens int
	// TriggerTokens is the estimate above which the frozen set is extended;
	// 0 never extends it, leaving the frozen set as the whole policy.
	TriggerTokens int
	// TargetTokens stops the extension: it walks candidates oldest-first only
	// until the estimate is at or below this. The gap between trigger and
	// target is the headroom that keeps the stub set — and with it the cached
	// prompt prefix — stable across many rounds.
	TargetTokens int
}

// Microcompact returns a provider-view projection of messages in which old
// oversized tool results are replaced with short metadata stubs, plus the
// stub set the next pass must keep applying. The session log is untouched —
// the caller sends the projection to the provider instead of the raw history —
// and each stub names its tool, keeps the output's first line and says how to
// recover the rest.
//
// The set is hysteresis, not a cache: every ID in frozen that still has a
// result in messages is stubbed again regardless of the current estimate, so
// the prefix the provider caches does not churn from round to round. Only
// when the projection still estimates above TriggerTokens does the pass
// extend the set, oldest candidate first, until the estimate reaches
// TargetTokens. IDs whose result has left the context — a macro compaction
// cut it away — are pruned from the returned set.
//
// A tool result is a candidate only when all of these hold:
//   - it sits before the last assistant message: everything the model has not
//     seen a reply to yet is the current round and rides verbatim, whatever
//     its size;
//   - it falls outside the keep-recent tail (KeepRecentTokens);
//   - p.KeepVerbatim reports false for its tool and arguments;
//   - its content reaches microFloorBytes;
//   - its tool call resolves (fail closed: keep everything else).
//
// The returned slice never aliases messages, on any path.
func Microcompact(
	messages []llm.Message,
	p MicroPolicy,
	frozen map[string]struct{},
) ([]llm.Message, MicroReport, map[string]struct{}) {
	projected := slices.Clone(messages)
	set := make(map[string]struct{}, len(frozen))
	report := MicroReport{}
	calls := toolCallsByID(messages)

	// stub replaces one result in place, reporting whether it did.
	stub := func(i int) bool {
		call, ok := calls[projected[i].ToolCallID]
		if !ok || len(projected[i].Content) < microFloorBytes {
			return false
		}
		text := microStub(call, projected[i].Content, p.Advice)
		report.Results++
		report.BytesElided += len(projected[i].Content) - len(text)
		projected[i].Content = text
		return true
	}

	// The frozen set first: it applies wherever its results still are.
	for i := range projected {
		if projected[i].Role != llm.RoleTool {
			continue
		}
		id := projected[i].ToolCallID
		if _, ok := frozen[id]; !ok {
			continue
		}
		set[id] = struct{}{}
		stub(i)
	}

	candidates := microCandidates(messages, calls, set, p)
	if p.TriggerTokens <= 0 || len(candidates) == 0 {
		return projected, report, set
	}
	estimate := 0
	for i := range projected {
		estimate += estimateMessageTokens(projected[i])
	}
	if estimate <= p.TriggerTokens {
		return projected, report, set
	}
	for _, i := range candidates {
		before := estimateMessageTokens(projected[i])
		if !stub(i) {
			continue
		}
		set[projected[i].ToolCallID] = struct{}{}
		estimate -= before - estimateMessageTokens(projected[i])
		if estimate <= p.TargetTokens {
			break
		}
	}
	return projected, report, set
}

// microCandidates lists the indexes of results the pass may newly stub, oldest
// first: outside the current round, outside the keep-recent tail, not already
// frozen, not kept verbatim, and big enough to be worth a stub.
func microCandidates(
	messages []llm.Message,
	calls map[string]llm.ToolCall,
	set map[string]struct{},
	p MicroPolicy,
) []int {
	if p.KeepVerbatim == nil {
		return nil
	}
	lastAssistant := -1
	for i := range messages {
		if messages[i].Role == llm.RoleAssistant {
			lastAssistant = i
		}
	}
	cutoff := min(microCutoff(messages, p.KeepRecentTokens), lastAssistant)
	var candidates []int
	for i := range max(cutoff, 0) {
		msg := messages[i]
		if msg.Role != llm.RoleTool || len(msg.Content) < microFloorBytes {
			continue
		}
		if _, frozen := set[msg.ToolCallID]; frozen {
			continue
		}
		call, ok := calls[msg.ToolCallID]
		if !ok || p.KeepVerbatim(call.Function.Name, call.Function.Arguments) {
			continue
		}
		candidates = append(candidates, i)
	}
	return candidates
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
// keeps the tool identity, the output's shape and its first line, so the
// model can tell what it lost, and closes with the recovery advice that fits
// the tool — re-running a mutating command is not a recovery.
func microStub(call llm.ToolCall, content string, advice func(tool, args string) string) string {
	tool := call.Function.Name
	lines := 0
	if content != "" {
		lines = strings.Count(content, "\n") + 1
	}
	sentence := ""
	if advice != nil {
		sentence = advice(tool, call.Function.Arguments)
	}
	if sentence == "" {
		sentence = "Re-run " + tool + " with the same arguments to recover the output."
	}
	var b strings.Builder
	b.WriteString("[tool result elided by microcompaction: ")
	b.WriteString(tool)
	b.WriteString(" returned ")
	writeCount(&b, lines, "line")
	b.WriteString(" (")
	writeCount(&b, len(content), "byte")
	b.WriteString(")")
	if first := microFirstLine(content); first != "" {
		b.WriteString("; first line: \"")
		b.WriteString(first)
		b.WriteString("\"")
	}
	b.WriteString(". ")
	b.WriteString(sentence)
	b.WriteString("]")
	return b.String()
}

// microFirstLine returns the first non-empty line of a tool result, bounded
// to microFirstLineBytes on a rune boundary. It is the one shred of output a
// stub keeps: an "@read path (…)" header, a listing's first row, an error's
// first sentence — enough for the model to recognize what was elided.
func microFirstLine(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) <= microFirstLineBytes {
			return line
		}
		cut := microFirstLineBytes
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		return line[:cut] + "…"
	}
	return ""
}

func writeCount(b *strings.Builder, n int, unit string) {
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" ")
	b.WriteString(unit)
	if n != 1 {
		b.WriteString("s")
	}
}
