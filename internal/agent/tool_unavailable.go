package agent

import (
	"encoding/json"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// rejectUnavailable leaves the human-facing reason intact: approval resume
// and transcript consumers key on it. Only the model receives the protocol.
func (e *Executor) rejectUnavailable(
	call llm.ToolCall,
	detail, reason, nextAction string,
	emit func(session.ToolData) bool,
) llm.Message {
	msg := e.rejectResult(call, detail, reason, emit)
	var result struct {
		Error struct {
			Code         string `json:"code"`
			Tool         string `json:"tool"`
			CurrentPhase Mode   `json:"current_phase"`
			Reason       string `json:"reason"`
			NextAction   string `json:"next_action"`
			RetryPolicy  string `json:"retry_policy"`
		} `json:"tool_error"`
	}
	result.Error.Code = "TOOL_NOT_AVAILABLE_IN_CURRENT_PHASE"
	result.Error.Tool = call.Function.Name
	result.Error.CurrentPhase = normalizeMode(e.mode)
	result.Error.Reason = reason
	result.Error.NextAction = nextAction
	result.Error.RetryPolicy = "This call was not executed. Follow next_action before retrying the same tool. " +
		"Do not substitute another tool, shell command, script, or delegation to perform the blocked action."
	// This closed payload contains only strings; JSON encoding cannot fail.
	content, _ := json.Marshal(result)
	msg.Content = string(content)
	return msg
}
