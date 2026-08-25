package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestInspectContextToolCallBody: an assistant turn whose content is empty
// because it carries tool calls is the model's tool_use block — the browser
// must show the call (name + arguments), not "(empty)". This is the shape of
// hundreds of rows in a real working session.
func TestInspectContextToolCallBody(t *testing.T) {
	m := newReportManager(t)
	appendMsg(t, m, llm.RoleUser, "look at the code")
	_, err := m.AppendAssistant(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "",
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Function: llm.Function{Name: "read", Arguments: `{"path": "internal/agent/engine.go"}`}},
			{ID: "call_2", Function: llm.Function{Name: "grep", Arguments: `{"pattern": "BuildContext"}`}},
		},
	}, "glm-5.2")
	require.NoError(t, err)

	report := m.InspectContext()
	require.Len(t, report.Items, 2)
	item := report.Items[1]

	assert.Equal(t, "assistant", item.Kind)
	assert.True(t, strings.HasPrefix(item.Preview, `read {"path":`),
		"preview shows the first tool call: %q", item.Preview)
	assert.Contains(t, item.Body, `read {"path": "internal/agent/engine.go"}`)
	assert.Contains(t, item.Body, `grep {"pattern": "BuildContext"}`,
		"the popup shows every call in the turn")
}

// TestInspectContextTextAndToolCallsBody: a turn with both prose and tool
// calls shows both — the popup must not hide what the turn called.
func TestInspectContextTextAndToolCallsBody(t *testing.T) {
	m := newReportManager(t)
	_, err := m.AppendAssistant(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "Checking the engine first.",
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Function: llm.Function{Name: "read", Arguments: `{"path": "a.go"}`}},
		},
	}, "glm-5.2")
	require.NoError(t, err)

	report := m.InspectContext()
	require.Len(t, report.Items, 1)
	assert.Equal(t, "Checking the engine first.", report.Items[0].Preview)
	assert.Contains(t, report.Items[0].Body, "Checking the engine first.")
	assert.Contains(t, report.Items[0].Body, `read {"path": "a.go"}`)
}

// TestInspectContextReasoningBody: a thinking-only assistant turn falls back
// to its reasoning text instead of rendering empty.
func TestInspectContextReasoningBody(t *testing.T) {
	m := newReportManager(t)
	_, err := m.AppendAssistant(llm.Message{
		Role:             llm.RoleAssistant,
		Content:          "",
		ReasoningContent: "We need inspect executor first.",
	}, "glm-5.2")
	require.NoError(t, err)

	report := m.InspectContext()
	require.Len(t, report.Items, 1)
	assert.Equal(t, "We need inspect executor first.", report.Items[0].Body)
	assert.Equal(t, "We need inspect executor first.", report.Items[0].Preview)
}
