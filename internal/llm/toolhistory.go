package llm

// InterruptedToolResult is the synthetic result used to close a tool call
// whose original agent run ended before a result was durably recorded.
const InterruptedToolResult = "Tool call did not complete because the previous agent run was interrupted."

// ToolHistoryRepair reports provider-context repairs. The append-only session
// remains an audit trail; callers use the repaired projection for inference.
type ToolHistoryRepair struct {
	InsertedResults int
	DroppedResults  int
}

// RepairToolHistory returns a provider-safe message projection. Every
// assistant tool call gets exactly one immediately following tool result;
// missing results become explicit interruption results, while orphan and
// duplicate results are excluded because providers reject them.
func RepairToolHistory(messages []Message) ([]Message, ToolHistoryRepair) {
	repaired := make([]Message, 0, len(messages))
	report := ToolHistoryRepair{}

	for i := 0; i < len(messages); {
		message := messages[i]
		if message.Role == RoleTool {
			report.DroppedResults++
			i++
			continue
		}
		if message.Role != RoleAssistant || len(message.ToolCalls) == 0 {
			repaired = append(repaired, message)
			i++
			continue
		}

		repaired = append(repaired, message)
		expected := make(map[string]struct{}, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			expected[call.ID] = struct{}{}
		}
		results := make(map[string]Message, len(message.ToolCalls))
		j := i + 1
		for j < len(messages) && messages[j].Role == RoleTool {
			result := messages[j]
			_, belongs := expected[result.ToolCallID]
			_, duplicate := results[result.ToolCallID]
			if belongs && !duplicate {
				results[result.ToolCallID] = result
			} else {
				report.DroppedResults++
			}
			j++
		}
		for _, call := range message.ToolCalls {
			if result, ok := results[call.ID]; ok {
				repaired = append(repaired, result)
				continue
			}
			repaired = append(repaired, Message{
				Role:       RoleTool,
				ToolCallID: call.ID,
				Content:    InterruptedToolResult,
			})
			report.InsertedResults++
		}
		i = j
	}

	return repaired, report
}

// PendingToolResults returns synthetic results needed to close only the
// trailing tool round. These results are safe to append durably because no
// later conversational message needs to be reordered.
func PendingToolResults(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	i := len(messages) - 1
	for i >= 0 && messages[i].Role == RoleTool {
		i--
	}
	if i < 0 || messages[i].Role != RoleAssistant || len(messages[i].ToolCalls) == 0 {
		return nil
	}

	provided := make(map[string]struct{}, len(messages)-i-1)
	for _, result := range messages[i+1:] {
		provided[result.ToolCallID] = struct{}{}
	}
	pending := make([]Message, 0, len(messages[i].ToolCalls))
	for _, call := range messages[i].ToolCalls {
		if _, ok := provided[call.ID]; ok {
			continue
		}
		pending = append(pending, Message{
			Role:       RoleTool,
			ToolCallID: call.ID,
			Content:    InterruptedToolResult,
		})
	}
	return pending
}
