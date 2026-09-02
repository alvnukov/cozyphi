package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func bigToolHistory() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: "inspect the logs"},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "b1",
				Type: "function",
				Function: llm.Function{
					Name:      "bash",
					Arguments: `{"command":"cat build.log"}`,
				},
			}},
		},
		{Role: llm.RoleTool, ToolCallID: "b1", Content: strings.Repeat("x", 100000)},
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

func TestProviderContextStubsUnderPressure(t *testing.T) {
	// Window 40000 → reminder threshold 23616; the 100 KB result estimates
	// past it, and keepRecentTokens=20000 leaves it outside the recent tail.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 40000)
	require.NoError(t, engine.session.Append(bigToolHistory()...))

	projected, report := engine.providerContext(engine.sessionRef())

	require.NotEqual(t, bigToolHistory()[2].Content, projected[2].Content)
	require.Contains(t, projected[2].Content, "bash returned")
	require.Equal(t, llm.RoleTool, projected[2].Role)
	require.Equal(t, 1, report.Results)
	require.Positive(t, report.BytesElided)

	// The durable session stays untouched: the next BuildContext still sees
	// the full result, so recovery by re-reading the log is not needed.
	require.Equal(t, bigToolHistory()[2].Content, engine.sessionRef().BuildContext()[2].Content)
}

func TestProviderContextKeepsAnchorCarryingResults(t *testing.T) {
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
				{ID: "b1", Type: "function", Function: llm.Function{Name: "bash", Arguments: `{"command":"ls"}`}},
			},
		},
		llm.Message{Role: llm.RoleTool, ToolCallID: "r1", Content: big},
		llm.Message{Role: llm.RoleTool, ToolCallID: "b1", Content: big},
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

func TestAnchorCarryingToolResult(t *testing.T) {
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
		{"bash", `{"command":"ls"}`, false},
		{"edit", `{"from":"1#a"}`, false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, anchorCarryingToolResult(tc.tool, tc.args), "%s %s", tc.tool, tc.args)
	}
}
