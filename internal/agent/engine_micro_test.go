package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func bashCall(id, command string) llm.ToolCall {
	return llm.ToolCall{
		ID:       id,
		Type:     "function",
		Function: llm.Function{Name: "bash", Arguments: `{"command":"` + command + `"}`},
	}
}

// bigToolHistory is one finished round — the model has already replied to the
// 100 KB result — followed by a fresh user turn.
func bigToolHistory() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: "inspect the logs"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{bashCall("b1", "cat build.log")}},
		{Role: llm.RoleTool, ToolCallID: "b1", Content: strings.Repeat("x", 100000)},
		{Role: llm.RoleAssistant, Content: "noted"},
		{Role: llm.RoleUser, Content: "continue"},
	}
}

func TestProviderContextUnderThresholdKeepsFidelity(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 1_000_000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.Equal(t, bigToolHistory(), projected)
	require.Equal(t, 0, report.Results)
}

func TestProviderContextKeepsCurrentRound(t *testing.T) {
	// Three parallel bash calls at the 50 KB output cap, answered in the round
	// the model has not seen yet: the next request must carry all of them.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	big := strings.Repeat("z", 50000)
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "inspect the logs"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			bashCall("b1", "cat a.log"), bashCall("b2", "cat b.log"), bashCall("b3", "cat c.log"),
		}},
		{Role: llm.RoleTool, ToolCallID: "b1", Content: big},
		{Role: llm.RoleTool, ToolCallID: "b2", Content: big},
		{Role: llm.RoleTool, ToolCallID: "b3", Content: big},
	}
	require.NoError(t, engine.session.Append(history...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.Equal(t, history, projected)
	require.Equal(t, 0, report.Results)
}

func TestProviderContextStubsUnderPressure(t *testing.T) {
	// Window 40000 → reminder threshold 23616; the 100 KB result estimates
	// past it, and keepRecentTokens=20000 leaves it outside the recent tail.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.NotEqual(t, bigToolHistory()[2].Content, projected[2].Content)
	require.Contains(t, projected[2].Content, "bash returned")
	require.Contains(t, projected[2].Content, "read-only", "bash advice must not invite a blind re-run")
	require.Equal(t, llm.RoleTool, projected[2].Role)
	require.Equal(t, 1, report.Results)
	require.Positive(t, report.BytesElided)

	// The durable session stays untouched: the next BuildContext still sees
	// the full result, so recovery by re-reading the log is not needed.
	require.Equal(t, bigToolHistory()[2].Content, engine.sessionRef().BuildContext()[2].Content)
}

func TestProviderContextFreezesStubs(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	first, firstReport := engine.providerContext(engine.sessionRef())
	require.Equal(t, 1, firstReport.Results)
	engine.mu.RLock()
	require.Contains(t, engine.microStubbed, "b1")
	engine.mu.RUnlock()

	// The same pressure yields the same projection, byte for byte: the cached
	// prompt prefix must not shift from round to round.
	second, secondReport := engine.providerContext(engine.sessionRef())
	require.Equal(t, first, second)
	require.Equal(t, 1, secondReport.Results)

	// Even once the pressure is gone the frozen stub stays applied — undoing
	// it would rewrite the prefix just as badly as adding one.
	engine.mu.Lock()
	engine.contextWindow = 1_000_000
	engine.mu.Unlock()
	calm, calmReport := engine.providerContext(engine.sessionRef())
	require.Equal(t, first, calm)
	require.Equal(t, 1, calmReport.Results)
	require.Equal(t, 1, engine.contextStats().MicroElidedResults)
}

func TestProviderContextTargetLeavesHeadroom(t *testing.T) {
	// Window 100000 → trigger 83616, target 73616. Four ~26000-token results
	// sit above the tail; two stubs clear the target, so the other two stay.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	big := strings.Repeat("q", 104000)
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "inspect the logs"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			bashCall("b1", "cat a.log"), bashCall("b2", "cat b.log"),
			bashCall("b3", "cat c.log"), bashCall("b4", "cat d.log"),
		}},
		{Role: llm.RoleTool, ToolCallID: "b1", Content: big},
		{Role: llm.RoleTool, ToolCallID: "b2", Content: big},
		{Role: llm.RoleTool, ToolCallID: "b3", Content: big},
		{Role: llm.RoleTool, ToolCallID: "b4", Content: big},
		{Role: llm.RoleAssistant, Content: "noted"},
		{Role: llm.RoleUser, Content: "continue"},
	}
	require.NoError(t, engine.session.Append(history...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.Equal(t, 2, report.Results, "the walk stops at the target, not at the last candidate")
	require.Contains(t, projected[2].Content, "bash returned", "oldest first")
	require.Contains(t, projected[3].Content, "bash returned")
	require.Equal(t, big, projected[4].Content, "headroom below the trigger keeps the set stable")
	require.Equal(t, big, projected[5].Content)
}

func TestProviderContextKeepsVerbatimResults(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	big := strings.Repeat("y", 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "inspect"},
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{
					ID:       "r1",
					Type:     "function",
					Function: llm.Function{Name: "read", Arguments: `{"path":"a.go","mode":"edit"}`},
				},
				bashCall("b1", "ls"),
			},
		},
		llm.Message{Role: llm.RoleTool, ToolCallID: "r1", Content: big},
		llm.Message{Role: llm.RoleTool, ToolCallID: "b1", Content: big},
		llm.Message{Role: llm.RoleAssistant, Content: "noted"},
		llm.Message{Role: llm.RoleUser, Content: "continue"},
	))

	projected, report := engine.providerContext(engine.sessionRef())

	require.Equal(t, big, projected[2].Content, "editable read must keep its anchors")
	require.NotEqual(t, big, projected[3].Content, "bash result is stubbed")
	require.Equal(t, 1, report.Results)
}

func TestProviderContextUnknownWindowKeepsFidelity(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 0)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.Equal(t, bigToolHistory(), projected)
	require.Equal(t, 0, report.Results)
}

func TestContextStatsReportsMicroElision(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	stats := engine.contextStats()
	require.Equal(t, 1, stats.MicroElidedResults)
	require.Positive(t, stats.MicroElidedBytes)

	calm := newContextTestEngine(t, "http://127.0.0.1:1", 1_000_000)
	require.NoError(t, calm.session.Append(bigToolHistory()...))
	require.Zero(t, calm.contextStats().MicroElidedResults)
}

func TestRearmCompactAdviceClearsMicroSet(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))
	_, report := engine.providerContext(engine.sessionRef())
	require.Equal(t, 1, report.Results)

	engine.rearmCompactAdvice()

	engine.mu.RLock()
	defer engine.mu.RUnlock()
	require.Empty(t, engine.microStubbed, "a landed compaction retires the frozen stub set")
}

func TestKeepToolResultVerbatim(t *testing.T) {
	cases := []struct {
		tool string
		args string
		want bool
	}{
		{"grep", `{"pattern":"x"}`, true},
		{"read", `{"path":"a.go","mode":"edit"}`, true},
		{"read", `{"path":"a.go","mode":"view"}`, false},
		{"read", `{"path":"a.go"}`, false},
		{"read", `not json`, true},
		{"question", `{"question":"which?"}`, true},
		{"agent_wait", `{"id":"a1"}`, true},
		{"bash", `{"command":"ls"}`, false},
		{"edit", `{"from":"1#a"}`, false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, keepToolResultVerbatim(tc.tool, tc.args), "%s %s", tc.tool, tc.args)
	}
}

func TestMicroAdvice(t *testing.T) {
	require.Contains(t, microAdvice("bash", `{"command":"rm -rf build"}`), "read-only")
	require.Contains(t, microAdvice("mcp_call", `{"tool":"deploy"}`), "read-only")
	require.Empty(t, microAdvice("read", `{"path":"a.go"}`), "other tools take the package generic sentence")
}
