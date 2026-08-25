package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// ToolCanceledResult is returned to the model when a user cancels a tool call.
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

	// planGate evaluates the approved-plan contract before the permission
	// gate. nil = no plan gating (children and unapproved sessions).
	planGate *plangate.Checker
	plan     func() session.Plan
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

// SetPlanGate attaches the approved-plan gate. A nil gate (or nil plan
// supplier) disables plan gating.
func (e *Executor) SetPlanGate(gate *plangate.Checker, plan func() session.Plan) {
	if e == nil {
		return
	}
	e.planGate = gate
	e.plan = plan
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
	results, _ := e.run(ctx, calls, emit)
	return results
}

func (e *Executor) run(
	ctx context.Context,
	calls []llm.ToolCall,
	emit func(session.ToolData) bool,
) ([]llm.Message, bool) {
	active := true
	send := func(data session.ToolData) bool {
		if !active {
			return false
		}
		active = emit(data)
		return active
	}

	results := make([]llm.Message, 0, len(calls))
	for _, call := range calls {
		if !active {
			// The event consumer is gone, so no further tool may run. Still
			// return one cancellation result for every advertised tool call:
			// the engine has already persisted the assistant tool_use message.
			results = append(results, e.cancelResult(call, send))
			continue
		}
		if ctx.Err() != nil {
			results = append(results, e.cancelResult(call, send))
			continue
		}
		results = append(results, e.runOne(ctx, call, send))
	}
	return results, active
}

func (e *Executor) runOne(ctx context.Context, call llm.ToolCall, emit func(session.ToolData) bool) llm.Message {
	ctx = tools.WithCwd(ctx, e.cwd)
	tool, ok := e.registry[call.Function.Name]
	args := json.RawMessage(call.Function.Arguments)
	detail := call.Function.Arguments
	planHint := ""
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

	// Plan gate: second gate between Pre hooks and the permission gate. Deny
	// blocks outright; Hint records the miss and appends guidance to the
	// model-facing result only (the TUI stays clean).
	if deny, hint := e.checkPlanGate(call, args); deny != "" {
		return e.rejectResult(call, detail, deny, emit)
	} else {
		planHint = hint
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

	if post.Output != "" {
		content = post.Output
		output = post.Output
	}

	// post.Stop is ignored until a later slice wires it into the agent loop.
	if planHint != "" {
		content = appendPlanGateHint(content, planHint)
	}
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
	req, err := permission.ExtractAt(call.Function.Name, args, e.cwd)
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

// checkPlanGate applies the approved-plan contract. It returns a deny reason
// (blocks the call) or a hint (appended to the model result).
func (e *Executor) checkPlanGate(call llm.ToolCall, args json.RawMessage) (deny, hint string) {
	if e.planGate == nil || e.plan == nil {
		return "", ""
	}
	plan := e.plan()
	step := planStepFromArgs(args)
	v := e.planGate.Check(plan, plangate.ToolCall{Name: call.Function.Name, PlanStep: step})
	if !v.Miss {
		return "", ""
	}
	if e.planGate.Recorder != nil {
		_ = e.planGate.Recorder.Record(e.missRecord(plan, call, step, v))
	}
	if v.Deny {
		return v.Reason, ""
	}
	return "", strings.TrimSpace(v.Reason + " " + v.Hint)
}

func (e *Executor) missRecord(plan session.Plan, call llm.ToolCall, step int, v plangate.Verdict) plangate.Miss {
	m := plangate.Miss{
		Session:      e.sessionID,
		Tool:         call.Function.Name,
		PlanStep:     step,
		PlanRevision: plan.Revision,
		Reason:       v.Reason,
		Phase:        string(e.planGate.Phase),
	}
	if step > 0 && step <= len(plan.Items) {
		item := plan.Items[step-1]
		m.StepStatus = string(item.Status)
		m.StepType = string(item.Type)
	}
	return m
}

func planStepFromArgs(args json.RawMessage) int {
	var in struct {
		PlanStep int `json:"plan_step"`
	}
	_ = json.Unmarshal(args, &in)
	return in.PlanStep
}

// appendPlanGateHint adds model-facing guidance without touching TUI output.
func appendPlanGateHint(content, hint string) string {
	if content == "" {
		return "[plan gate] " + hint
	}
	return content + "\n\n[plan gate] " + hint
}

// toolRun builds a ToolData payload with Name always set so headless JSONL
// and stderr logs never omit toolName.
func (*Executor) toolRun(
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

func (*Executor) toolMessage(id, content string) llm.Message {
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
