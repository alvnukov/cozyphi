package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

const ToolCanceledResult = "User cancelled the tool call."

// Executor runs model tool_calls against a tool registry and emits ToolData for the UI.
type Executor struct {
	registry tools.Registry
	gate     permission.Gate
	ask      permission.AskFunc
}

func NewExecutor(registry tools.Registry, gate permission.Gate, ask permission.AskFunc) *Executor {
	if gate == nil {
		gate = permission.AllowAll{}
	}
	return &Executor{registry: registry, gate: gate, ask: ask}
}

// Run executes tool calls in order, yielding ToolData updates via emit.
// Returns role=tool messages for the next LLM turn (including cancel stubs).
func (e *Executor) Run(
	ctx context.Context,
	calls []llm.ToolCall,
	emit func(session.ToolData) bool,
) []llm.Message {
	results := make([]llm.Message, 0, len(calls))
	for _, call := range calls {
		if ctx.Err() != nil {
			results = append(results, e.cancelResult(call, emit))
			continue
		}
		results = append(results, e.runOne(ctx, call, emit))
	}
	return results
}

func (e *Executor) runOne(ctx context.Context, call llm.ToolCall, emit func(session.ToolData) bool) llm.Message {
	tool, ok := e.registry[call.Function.Name]
	args := json.RawMessage(call.Function.Arguments)
	detail := call.Function.Arguments
	if ok && tool.DetailFromArgs != nil {
		if d := tool.DetailFromArgs(args); d != "" {
			detail = d
		}
	}

	if !emit(session.ToolData{Run: e.toolRun(call, session.ToolInProgress, detail, "", "")}) {
		return e.toolMessage(call.ID, ToolCanceledResult)
	}

	if !ok {
		errText := fmt.Sprintf("tool '%s' not found", call.Function.Name)
		_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolError, detail, errText, "")})
		return e.toolMessage(call.ID, errText)
	}

	if msg, rejected := e.checkPermission(ctx, call, args, detail, emit); rejected {
		return msg
	}

	result, err := tool.Run(tools.WithToolCallID(ctx, call.ID), args)
	if err != nil {
		if ctx.Err() != nil {
			return e.cancelResult(call, emit)
		}
		errText := err.Error()
		_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolError, detail, errText, errText)})
		return e.toolMessage(call.ID, errText)
	}

	out := result.Output
	if out == "" {
		out = result.Content
	}
	if result.Detail != "" {
		detail = result.Detail
	}
	_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolDone, detail, "", out)})
	return e.toolMessage(call.ID, result.Content)
}

func (e *Executor) checkPermission(
	ctx context.Context,
	call llm.ToolCall,
	args json.RawMessage,
	detail string,
	emit func(session.ToolData) bool,
) (llm.Message, bool) {
	req, err := permission.Extract(call.Function.Name, args)
	if err != nil {
		reason := fmt.Sprintf("permission check failed: %v", err)
		return e.rejectResult(call, detail, reason, emit), true
	}

	dec, reason := e.gate.Check(ctx, req)
	switch dec {
	case permission.Allow:
		return llm.Message{}, false
	case permission.Deny:
		if reason == "" {
			reason = "tool execution denied by permissions"
		}
		return e.rejectResult(call, detail, reason, emit), true
	case permission.Ask:
		if e.ask == nil {
			if reason == "" {
				reason = "tool requires approval but no ask handler is configured"
			}
			return e.rejectResult(call, detail, reason, emit), true
		}
		res, askErr := e.ask(ctx, req, reason)
		if askErr != nil {
			msg := fmt.Sprintf("approval failed: %v", askErr)
			return e.rejectResult(call, detail, msg, emit), true
		}
		if !res.Approved {
			msg := "tool execution rejected by user"
			if res.Feedback != "" {
				msg = "This tool call was rejected by the user with feedback: " + res.Feedback
			}
			return e.rejectResult(call, detail, msg, emit), true
		}
		return llm.Message{}, false
	default:
		return e.rejectResult(call, detail, "unknown permission decision", emit), true
	}
}

func (e *Executor) rejectResult(call llm.ToolCall, detail, reason string, emit func(session.ToolData) bool) llm.Message {
	_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolRejected, detail, reason, reason)})
	return e.toolMessage(call.ID, reason)
}

func (e *Executor) cancelResult(call llm.ToolCall, emit func(session.ToolData) bool) llm.Message {
	detail := call.Function.Arguments
	if tool, ok := e.registry[call.Function.Name]; ok && tool.DetailFromArgs != nil {
		if d := tool.DetailFromArgs(json.RawMessage(call.Function.Arguments)); d != "" {
			detail = d
		}
	}
	_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolCancelled, detail, "", ToolCanceledResult)})
	return e.toolMessage(call.ID, ToolCanceledResult)
}

// toolRun builds a ToolData payload with Name always set so headless JSONL
// and stderr logs never omit toolName.
func (e *Executor) toolRun(call llm.ToolCall, status session.ToolStatus, detail, errText, output string) session.ToolRun {
	return session.ToolRun{
		ToolUseID: call.ID,
		Name:      call.Function.Name,
		Status:    status,
		Detail:    detail,
		Error:     errText,
		Output:    output,
	}
}

func (e *Executor) toolMessage(id, content string) llm.Message {
	return llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: id,
		Content:    content,
	}
}
