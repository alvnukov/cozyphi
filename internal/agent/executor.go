package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools"
)

const ToolCanceledResult = "User cancelled the tool call."

const (
	hookContextOpen  = "<hook_context>"
	hookContextClose = "</hook_context>"
)

// Executor runs model tool_calls against a tool registry and emits ToolData for the UI.
type Executor struct {
	registry  tools.Registry
	gate      permission.Gate
	ask       permission.AskFunc
	hooks     *hooks.Manager // nil = no hooks (behavior identical to pre-hooks)
	sessionID string
	cwd       string

	// failClosedHooksOnly is set in ModeReadonly: only FailClosed hooks run
	// so slow audit hooks cannot stall exploration.
	failClosedHooksOnly bool
}

// NewExecutor builds an executor. hookMgr may be nil.
func NewExecutor(
	registry tools.Registry,
	gate permission.Gate,
	ask permission.AskFunc,
	hookMgr *hooks.Manager,
) *Executor {
	if gate == nil {
		gate = permission.AllowAll{}
	}
	e := &Executor{registry: registry, gate: gate, ask: ask, hooks: hookMgr}
	e.syncHookFilter()
	return e
}

// SetMeta attaches session identity used in hook Event payloads.
func (e *Executor) SetMeta(sessionID, cwd string) {
	if e == nil {
		return
	}
	e.sessionID = sessionID
	e.cwd = cwd
}

func (e *Executor) syncHookFilter() {
	if e == nil {
		return
	}
	e.failClosedHooksOnly = permission.ModeOf(e.gate) == permission.ModeReadonly
}

func (e *Executor) activeHooks() *hooks.Manager {
	if e == nil || e.hooks == nil {
		return nil
	}
	if e.failClosedHooksOnly {
		return e.hooks.FailClosedOnly()
	}
	return e.hooks
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

	// Pre → Gate → Run → Post. Pre runs before permission Ask so org policy
	// can deny without prompting the user.
	pre := e.activeHooks().PreTool(ctx, hooks.Event{
		SessionID: e.sessionID,
		Cwd:       e.cwd,
		Tool:      call.Function.Name,
		ToolUseID: call.ID,
		Input:     args,
	})
	if pre.Denied {
		reason := pre.Reason
		if reason == "" {
			reason = "tool execution denied by hook"
		}
		reason = appendHookContext(reason, pre.Context)
		return e.rejectResult(call, detail, reason, emit)
	}
	if len(pre.Input) > 0 {
		args = pre.Input
		if tool.DetailFromArgs != nil {
			if d := tool.DetailFromArgs(args); d != "" {
				detail = d
			}
		} else {
			detail = string(args)
		}
	}

	if msg, rejected := e.checkPermission(ctx, call, args, detail, emit); rejected {
		return msg
	}

	result, err := tool.Run(tools.WithToolCallID(ctx, call.ID), args)

	var (
		errText string
		content string
		output  string
	)
	if err != nil {
		if ctx.Err() != nil {
			return e.cancelResult(call, emit)
		}
		errText = err.Error()
		content = errText
		output = errText
	} else {
		content = result.Content
		output = result.Output
		if output == "" {
			output = result.Content
		}
		if result.Detail != "" {
			detail = result.Detail
		}
	}

	post := e.activeHooks().PostTool(ctx, hooks.Event{
		SessionID: e.sessionID,
		Cwd:       e.cwd,
		Tool:      call.Function.Name,
		ToolUseID: call.ID,
		Input:     args,
		Output:    output,
		Err:       errText,
	})
	// post.Stop is ignored until a later slice wires it into the agent loop.
	modelContent := appendHookContext(content, joinHookContexts(pre.Context, post.Context))

	if err != nil {
		_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolError, detail, errText, output)})
		return e.toolMessage(call.ID, modelContent)
	}
	_ = emit(session.ToolData{Run: e.toolRun(call, session.ToolDone, detail, "", output)})
	return e.toolMessage(call.ID, modelContent)
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

func (e *Executor) rejectResult(
	call llm.ToolCall,
	detail, reason string,
	emit func(session.ToolData) bool,
) llm.Message {
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
func (e *Executor) toolRun(
	call llm.ToolCall,
	status session.ToolStatus,
	detail, errText, output string,
) session.ToolRun {
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

func joinHookContexts(parts ...string) string {
	var nonempty []string
	for _, p := range parts {
		if p != "" {
			nonempty = append(nonempty, p)
		}
	}
	return strings.Join(nonempty, "\n\n")
}

// appendHookContext adds model-facing hook notes. TUI Detail/Output stay clean.
// Closing tags inside ctx are escaped so the model cannot break out of the block.
func appendHookContext(content, ctx string) string {
	if ctx == "" {
		return content
	}
	escaped := strings.ReplaceAll(ctx, hookContextClose, "</hook_context\u200b>")
	block := hookContextOpen + "\n" + escaped + "\n" + hookContextClose
	if content == "" {
		return block
	}
	return content + "\n\n" + block
}
