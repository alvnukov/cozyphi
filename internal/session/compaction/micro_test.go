package compaction

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// assistantWithBashCalls is one assistant turn firing several bash calls at
// once — the parallel round the current-round rule must keep verbatim.
func assistantWithBashCalls(ids ...string) llm.Message {
	msg := llm.Message{Role: llm.RoleAssistant}
	for _, id := range ids {
		msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
			ID:   id,
			Type: "function",
			Function: llm.Function{
				Name:      "bash",
				Arguments: `{"command":"cat big.log"}`,
			},
		})
	}
	return msg
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

// stubEverything is the policy the size tests use: nothing is kept verbatim,
// the tail is minimal and the trigger fires on any history at all.
func stubEverything(keepRecent int) MicroPolicy {
	return MicroPolicy{
		KeepVerbatim:     stubNothing,
		KeepRecentTokens: keepRecent,
		TriggerTokens:    1,
	}
}

// parallelRound is one user turn answered by three 50 KB bash results — the
// bash output cap, three ways, in a single round.
func parallelRound() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: "look at the repo"},
		assistantWithBashCalls("b1", "b2", "b3"),
		toolResult("b1", bigContent(50000)),
		toolResult("b2", bigContent(50000)),
		toolResult("b3", bigContent(50000)),
	}
}

func TestMicrocompactKeepsCurrentRound(t *testing.T) {
	messages := parallelRound()

	projected, report, set := Microcompact(messages, stubEverything(1), nil)

	assert.Equal(t, messages, projected, "the round the model has not seen yet rides verbatim")
	assert.Equal(t, MicroReport{}, report)
	assert.Empty(t, set)
}

func TestMicrocompactStubsRoundsTheModelHasSeen(t *testing.T) {
	messages := append(parallelRound(),
		llm.Message{Role: llm.RoleAssistant, Content: "noted"},
		llm.Message{Role: llm.RoleUser, Content: "continue"},
	)

	projected, report, set := Microcompact(messages, stubEverything(1), nil)

	for i := 2; i <= 4; i++ {
		assert.Contains(t, projected[i].Content, "bash returned", "result %d must be stubbed", i)
		assert.Equal(t, llm.RoleTool, projected[i].Role)
	}
	assert.Equal(t, bigContent(50000), messages[2].Content, "input slice must stay untouched")
	assert.Equal(t, messages[1], projected[1], "assistant tool calls stay verbatim")
	assert.Equal(t, messages[6], projected[6], "the tail stays verbatim")
	assert.Equal(t, 3, report.Results)
	assert.Positive(t, report.BytesElided)
	assert.Equal(t, map[string]struct{}{"b1": {}, "b2": {}, "b3": {}}, set)
}

func TestMicrocompactAppliesFrozenSetBelowTrigger(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "look"},
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}
	// No candidates at all: the trigger never fires and nothing is stubbable.
	policy := MicroPolicy{KeepRecentTokens: 1_000_000}

	projected, report, set := Microcompact(messages, policy, map[string]struct{}{"b1": {}})

	assert.Contains(t, projected[2].Content, "bash returned", "a frozen stub outlives the pressure that made it")
	assert.Equal(t, 1, report.Results)
	assert.Equal(t, map[string]struct{}{"b1": {}}, set)
}

func TestMicrocompactExtensionStopsAtTarget(t *testing.T) {
	messages := append(parallelRound(),
		llm.Message{Role: llm.RoleAssistant, Content: "noted"},
		llm.Message{Role: llm.RoleUser, Content: "continue"},
	)
	// ~37600 estimated tokens; stubbing the oldest result alone drops ~12400,
	// which lands under the target with two candidates still untouched.
	policy := MicroPolicy{
		KeepVerbatim:     stubNothing,
		KeepRecentTokens: 100,
		TriggerTokens:    30000,
		TargetTokens:     27000,
	}

	projected, report, set := Microcompact(messages, policy, nil)

	assert.Contains(t, projected[2].Content, "bash returned", "the oldest candidate goes first")
	assert.Equal(t, bigContent(50000), projected[3].Content, "the target stops the walk")
	assert.Equal(t, bigContent(50000), projected[4].Content)
	assert.Equal(t, 1, report.Results)
	assert.Equal(t, map[string]struct{}{"b1": {}}, set)
}

func TestMicrocompactPrunesStaleFrozenIDs(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "look"},
		{Role: llm.RoleAssistant, Content: "compacted away"},
	}

	projected, report, set := Microcompact(messages, stubEverything(1), map[string]struct{}{"gone": {}})

	assert.Equal(t, messages, projected)
	assert.Equal(t, MicroReport{}, report)
	assert.Empty(t, set, "an ID whose result left the context is pruned")
}

func TestMicrocompactNeverAliasesInput(t *testing.T) {
	quiet := []llm.Message{{Role: llm.RoleUser, Content: "hello"}}

	projected, _, _ := Microcompact(quiet, MicroPolicy{}, nil)
	require.Len(t, projected, 1)
	projected[0].Content = "mutated"
	assert.Equal(t, "hello", quiet[0].Content, "the no-op path must not hand back the input slice")

	busy := append(parallelRound(),
		llm.Message{Role: llm.RoleAssistant, Content: "noted"},
	)
	stubbed, report, _ := Microcompact(busy, stubEverything(1), nil)
	require.Equal(t, 3, report.Results)
	stubbed[0].Content = "mutated"
	assert.Equal(t, "look at the repo", busy[0].Content, "the stubbing path must not alias either")
}

func TestMicrocompactKeepsRecentWindow(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "look"},
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}

	projected, report, set := Microcompact(messages, stubEverything(1_000_000), nil)

	assert.Equal(t, messages, projected)
	assert.Equal(t, MicroReport{}, report)
	assert.Empty(t, set)
}

func TestMicrocompactKeepsVerbatimResults(t *testing.T) {
	messages := []llm.Message{
		assistantWithCall("g1", "grep", `{"pattern":"x"}`),
		toolResult("g1", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}
	policy := stubEverything(1)
	policy.KeepVerbatim = keepGrep

	projected, report, set := Microcompact(messages, policy, nil)

	assert.Equal(t, bigContent(10000), projected[1].Content, "anchored results must stay verbatim")
	assert.Equal(t, MicroReport{}, report)
	assert.Empty(t, set)
}

func TestMicrocompactKeepsSmallResults(t *testing.T) {
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(100)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}

	projected, report, _ := Microcompact(messages, stubEverything(1), nil)

	assert.Equal(t, bigContent(100), projected[1].Content)
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactNilPredicateStubsNothing(t *testing.T) {
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}
	policy := stubEverything(1)
	policy.KeepVerbatim = nil

	projected, report, set := Microcompact(messages, policy, nil)

	assert.Equal(t, messages, projected)
	assert.Equal(t, MicroReport{}, report)
	assert.Empty(t, set)
}

func TestMicrocompactUnresolvedCallKeepsResult(t *testing.T) {
	messages := []llm.Message{
		toolResult("orphan", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}

	projected, report, _ := Microcompact(messages, stubEverything(1), nil)

	assert.Equal(t, bigContent(10000), projected[0].Content, "unresolvable calls fail closed")
	assert.Equal(t, MicroReport{}, report)
}

func TestMicrocompactIsIdempotent(t *testing.T) {
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		toolResult("b1", bigContent(10000)),
		{Role: llm.RoleAssistant, Content: "noted"},
	}

	first, firstReport, firstSet := Microcompact(messages, stubEverything(1), nil)
	second, _, secondSet := Microcompact(first, stubEverything(1), firstSet)

	assert.Equal(t, first, second)
	assert.Equal(t, firstSet, secondSet)
	assert.Equal(t, 1, firstReport.Results)
}

func TestMicrocompactKeepsMedia(t *testing.T) {
	result := toolResult("b1", bigContent(10000))
	result.Media = []llm.Media{{MediaType: "image/png", Data: "AAAA"}}
	messages := []llm.Message{
		assistantWithCall("b1", "bash", "{}"),
		result,
		{Role: llm.RoleAssistant, Content: "noted"},
	}

	projected, report, _ := Microcompact(messages, stubEverything(1), nil)

	assert.Equal(t, 1, report.Results)
	assert.Equal(t, result.Media, projected[1].Media, "media survives the stub")
	assert.Equal(t, "b1", projected[1].ToolCallID)
}

func readCall(args string) llm.ToolCall {
	return llm.ToolCall{ID: "r1", Type: "function", Function: llm.Function{Name: "read", Arguments: args}}
}

func TestMicroStubKeepsFirstLineAndGenericAdvice(t *testing.T) {
	stub := microStub(readCall(`{"path":"x.go"}`), "@read x.go (2400 lines)\nfirst\nsecond", nil)

	assert.Contains(t, stub, "read returned 3 lines")
	assert.Contains(t, stub, `first line: "@read x.go (2400 lines)"`)
	assert.Contains(t, stub, "Re-run read with the same arguments to recover the output.")
}

func TestMicroStubCountsSingulars(t *testing.T) {
	stub := microStub(readCall("{}"), "x", nil)

	assert.Contains(t, stub, "read returned 1 line (1 byte)")
}

func TestMicroStubUsesPolicyAdvice(t *testing.T) {
	advice := func(tool, args string) string {
		assert.Equal(t, "read", tool)
		assert.JSONEq(t, `{"path":"x.go"}`, args)
		return "Work from what you concluded."
	}

	stub := microStub(readCall(`{"path":"x.go"}`), "line\n", advice)

	assert.Contains(t, stub, "Work from what you concluded.")
	assert.NotContains(t, stub, "Re-run read")
}

func TestMicroFirstLineIsRuneSafe(t *testing.T) {
	long := microFirstLine(strings.Repeat("é", 300))
	assert.Equal(t, strings.Repeat("é", 100)+"…", long)
	assert.True(t, utf8.ValidString(long))

	assert.Equal(t, "second", microFirstLine("\n   \nsecond\nthird"))
	assert.Empty(t, microFirstLine("\n \n"))
}
