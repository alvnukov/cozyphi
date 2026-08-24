package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepairToolHistoryInsertsMissingResultsBeforeNextMessage(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "run tools"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "call_1"},
			{ID: "call_2"},
			{ID: "call_3"},
		}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "done"},
		{Role: RoleUser, Content: "continue"},
	}

	repaired, report := RepairToolHistory(messages)

	require.Equal(t, ToolHistoryRepair{InsertedResults: 2}, report)
	require.Len(t, repaired, 6)
	require.Equal(t, "call_1", repaired[2].ToolCallID)
	require.Equal(t, "call_2", repaired[3].ToolCallID)
	require.Equal(t, InterruptedToolResult, repaired[3].Content)
	require.Equal(t, "call_3", repaired[4].ToolCallID)
	require.Equal(t, RoleUser, repaired[5].Role)
}

func TestRepairToolHistoryDropsOrphanAndDuplicateResults(t *testing.T) {
	messages := []Message{
		{Role: RoleTool, ToolCallID: "orphan", Content: "unsafe"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1"}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "first"},
		{Role: RoleTool, ToolCallID: "call_1", Content: "duplicate"},
	}

	repaired, report := RepairToolHistory(messages)

	require.Equal(t, ToolHistoryRepair{DroppedResults: 2}, report)
	require.Equal(t, []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1"}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "first"},
	}, repaired)
}

func TestPendingToolResultsReturnsOnlyMissingTrailingResults(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "run tools"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "done"},
	}

	require.Equal(t, []Message{{
		Role:       RoleTool,
		ToolCallID: "call_2",
		Content:    InterruptedToolResult,
	}}, PendingToolResults(messages))
	require.Nil(t, PendingToolResults(append(messages, Message{Role: RoleUser, Content: "later"})))
}
