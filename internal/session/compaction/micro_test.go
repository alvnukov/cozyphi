package compaction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func bigContent(n int) string {
	return strings.Repeat("x", n)
}

func assistantWithCall(id, tool, args string) llm.Message {
	return llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID:   id,
			Type: "function",
			Function: llm.Function{
				Name:      tool,
				Arguments: args,
			},
		}},
	}
}

func toolResult(id, content string) llm.Message {
	return llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: id,
		Content:    content,
	}
}

func stubNothing(_, _ string) bool { return false }

func keepGrep(tool, _ string) bool { return tool == "grep" }

func TestMicrocompactStubsOldOversizedResults(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "look at the repo"},
		assistantWithCall("b1", "bash", `{"command":"cat big.log"}`),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	projected, report := Microcompact(messages, s, stubNothing)

	assert.Equal(t, bigContent(10000), messages[2].Content, "input slice must stay untouched")
	assert.NotEqual(t, bigContent(10000), projected[2].Content, "old oversized result must be stubbed")
	assert.Contains(t, projected[2].Content, "bash")
	assert.Contains(t, projected[2].Content, "10000 bytes")
	assert.Equal(t, llm.RoleTool, projected[2].Role)
	assert.Equal(t, "b1", projected[2].ToolCallID)
	assert.Equal(t, messages[1], projected[1], "assistant tool calls must stay verbatim")
	assert.Equal(t, messages[3], projected[3], "recent tail must stay verbatim")
	assert.Equal(t, 1, report.Results)
	assert.Equal(t, 10000-len(projected[2].Content), report.BytesElided)
}

func TestMicrocompactKeepsRecentWindow(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 1_000_000}
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "look"},
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
	}

	projected, report := Microcompact(messages, s, stubNothing)

	assert.Equal(t, messages, projected)
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactKeepsAnchorCarryingResults(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		assistantWithCall("g1", "grep", `{"pattern":"x"}`),
		toolResult("g1", bigContent(10000)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	projected, report := Microcompact(messages, s, keepGrep)

	assert.Equal(t, bigContent(10000), projected[1].Content, "anchored results must stay verbatim")
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactKeepsSmallResults(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(100)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	projected, report := Microcompact(messages, s, stubNothing)

	assert.Equal(t, bigContent(100), projected[1].Content)
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactNilPredicateIsNoop(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	projected, report := Microcompact(messages, s, nil)

	assert.Equal(t, messages, projected)
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactUnresolvedCallKeepsResult(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		toolResult("orphan", bigContent(10000)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	projected, report := Microcompact(messages, s, stubNothing)

	assert.Equal(t, bigContent(10000), projected[0].Content, "unresolvable calls fail closed")
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactIsIdempotent(t *testing.T) {
	s := Settings{enabled: true, keepRecentTokens: 32}
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleUser, Content: "tail turn"},
	}

	first, firstReport := Microcompact(messages, s, stubNothing)
	second, secondReport := Microcompact(first, s, stubNothing)

	assert.Equal(t, first, second)
	assert.Equal(t, MicroReport{}, secondReport)
	assert.Equal(t, 1, firstReport.Results)
}

func TestMicrocompactStubCountsLines(t *testing.T) {
	stub := microStub("bash", "a\nb\nc")
	assert.Contains(t, stub, "bash returned 3 lines")
	assert.Contains(t, stub, "5 bytes")
	assert.Contains(t, stub, "re-run bash to recover")

	single := microStub("read", "x")
	assert.Contains(t, single, "read returned 1 line (1 byte)")
}
