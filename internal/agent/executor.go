package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
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
	// startStep moves one pending step to in_progress for a call that cleared
	// every gate; nil = the verdict's StartPending is never applied (tests,
	// and any engine wired without a session).
	startStep func(ctx context.Context, stepID string) error
	// settlePlan applies one piggybacked _plan settle atomically — complete
	// the previous step, swap the working context, start the target; nil =
	// calls carrying _plan are rejected instead of half-applied.
	settlePlan func(ctx context.Context, settle session.PlanSettle) error
	// recordStep files one bounded attempt on the step the verdict named;
	// nil = accepted calls leave no plan evidence.
	recordStep func(stepID string, attempt session.PlanAttempt) error
	// approveStep durably records the user's just-in-time verdict for one
	// step; nil = an approved handoff runs the call without a durable grant.
	approveStep func(stepID string, granted bool) error
	// planMiss and planOnlyRound feed the session's plan observability
	// counters from the executor's own decision points; nil = counting off
	// (tests, sub-agents, plan-disabled engines).
	planMiss      func()
	planOnlyRound func()

	// compactGate is the engine's hard-compaction gate: a non-empty
	// directive for a tool name blocks the call before hooks and permissions
	// spend anything. nil = no gate.
	compactGate func(tool string) string
	// drainCompactAdvice, when wired, moves a compaction recommendation a
	// call parked mid-run (its settle transition or the plan tool's own
	// action) into that call's result, so the model meets it at this
	// boundary instead of one prompt later. nil = advice waits for the next
	// prompt.
	drainCompactAdvice func() string
	// drainPlanSkills, when wired, returns plan-step skill text queued at a
	// step_start boundary and whether it is guidance the model has not seen.
	// Only such guidance refuses the starting call; a reminder of skills
	// already in context rides the result instead of costing a round. nil
	// leaves queued content for the next composed prompt.
	drainPlanSkills func() (string, bool)
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

// SetPlanGate attaches the approved-plan gate together with the callbacks that
// apply its verdicts: after a gateable call clears the plan gate and the
// permission gate, start moves the pending step the verdict named to
// in_progress before dispatch, settle applies the plan metadata a call
// piggybacked through _plan (completing the previous step, swapping the
// working context and starting the named step as one atomic write), record
// files the call's bounded attempt evidence on the step the verdict resolved
// once the call reaches a terminal outcome, and approve durably records the
// user's just-in-time verdict when the handoff asked for one. A nil gate (or
// nil plan supplier) disables plan gating; nil callbacks leave the
// corresponding verdict half unapplied while the call still runs. Wiring them
// as one call keeps a gate from ever running without its appliers.
func (e *Executor) SetPlanGate(
	gate *plangate.Checker,
	plan func() session.Plan,
	start func(ctx context.Context, stepID string) error,
	settle func(ctx context.Context, settle session.PlanSettle) error,
	record func(stepID string, attempt session.PlanAttempt) error,
	approve func(stepID string, granted bool) error,
) {
	if e == nil {
		return
	}
	e.planGate = gate
	e.plan = plan
	e.startStep = start
	e.settlePlan = settle
	e.recordStep = record
	e.approveStep = approve
}

// SetPlanTelemetry wires the plan observability counters the executor owns:
// miss fires when the plan gate refuses a call for addressing, planOnlyRound
// fires when a model round carries no working call. nil = counting off; the
// gates and appliers are unaffected either way.
func (e *Executor) SetPlanTelemetry(miss, planOnlyRound func()) {
	if e == nil {
		return
	}
	e.planMiss = miss
	e.planOnlyRound = planOnlyRound
}

// SetCompactGate wires the engine's hard-compaction gate: in hard mode the
// engine returns a directive for every tool except the context tool, and the
// executor refuses the call with it. nil disables the gate.
func (e *Executor) SetCompactGate(gate func(tool string) string) {
	if e == nil {
		return
	}
	e.compactGate = gate
}

// SetCompactAdviceDrain wires the engine's parked-compaction-advice drain.
// The executor calls it once a tool has run, so advice the call itself
// parked rides that call's result instead of the next user prompt. nil
// keeps the next-prompt delivery.
func (e *Executor) SetCompactAdviceDrain(drain func() string) {
	if e == nil {
		return
	}
	e.drainCompactAdvice = drain
}

// SetPlanSkillDrain wires the engine's parked plan-skill preload. The executor
// checks it after settling/starting and again after Run; nil keeps next-prompt
// delivery for steps started outside a tool call.
func (e *Executor) SetPlanSkillDrain(drain func() (string, bool)) {
	if e == nil {
		return
	}
	e.drainPlanSkills = drain
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

// hookStop is a PostTool hook's request to end the run. The zero value is
// "no hook asked", which an empty reason string could not say: a hook may
// stop without giving a reason, and that is not the same as not stopping.
type hookStop struct {
	stopped bool
	reason  string
}

// defaultHookStopReason stands in for a hook that stopped without saying
// why. It is written once: the model reminder and the run's error both read
// it from here rather than comparing text.
const defaultHookStopReason = "post-tool hook requested stop"

// Reason is what the user and the model are told, the hook's own words when
// it gave any.
func (s hookStop) Reason() string {
	if !s.stopped {
		return ""
	}
	if s.reason == "" {
		return defaultHookStopReason
	}
	return s.reason
}

// Err is the loop error for this stop, nil when no hook asked for one. A
// hook that named a reason has it wrapped into the error; one that did not
// leaves ErrPostHookStop to speak for itself.
func (s hookStop) Err() error {
	switch {
	case !s.stopped:
		return nil
	case s.reason == "":
		return ErrPostHookStop
	default:
		return fmt.Errorf("%w: %s", ErrPostHookStop, s.reason)
	}
}

// roundState is what one tool round carries from call to call: a skill
// preload that has to be retried before anything else runs, and a hook's
// request to stop.
type roundState struct {
	skillRetryRequired bool
	stop               hookStop
}

func (e *Executor) run(
	ctx context.Context,
	calls []llm.ToolCall,
	emit func(session.ToolData) bool,
) ([]llm.Message, bool, hookStop) {
	// A round whose every call is plan-side (plan, question, watch — the
	// exempt set) advanced no step: budget spent purely on the plan itself.
	if len(calls) > 0 && e.planGate != nil && e.planOnlyRound != nil {
		onlyPlan := true
		for _, call := range calls {
			if !plangate.IsExempt(call.Function.Name) {
				onlyPlan = false
				break
			}
		}
		if onlyPlan {
			e.planOnlyRound()
		}
	}
	active := true
	send := func(data session.ToolData) bool {
		if !active {
			return false
		}
		active = emit(data)
		return active
	}

	results := make([]llm.Message, 0, len(calls))
	// state carries a PostTool hook's Stop out of the round: the engine ends
	// the turn with it instead of sending the next model request.
	state := roundState{}
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
		if state.stop.stopped {
			// The hook stop ends the round's execution; the advertised calls
			// still get results so tool_use/result pairing survives.
			results = append(results, e.cancelResult(call, send))
			continue
		}
		if state.skillRetryRequired {
			results = append(results,
				e.rejectResult(call, call.Function.Arguments, plangate.ReasonBatchSkillPreload, send))
			continue
		}
		results = append(results, e.runOne(ctx, call, send, &state))
	}
	return results, active, state.stop
}

func (e *Executor) runOne(
	ctx context.Context,
	call llm.ToolCall,
	emit func(session.ToolData) bool,
	state *roundState,
) llm.Message {
	ctx = tools.WithCwd(ctx, e.cwd)
	tool, ok := e.registry[call.Function.Name]
	args := json.RawMessage(call.Function.Arguments)
	// The harness-owned _plan envelope leaves the arguments before anything
	// else reads them: hooks, the permission gate and the tool's own strict
	// decode all judge the call's own arguments only. A malformed envelope
	// rejects the whole call — the model clearly meant to settle something,
	// and dropping it silently would desynchronize the plan from the
	// transcript.
	envelope, cleanedArgs, err := plangate.SplitEnvelope(args)
	if err != nil {
		return e.rejectResult(call, call.Function.Arguments, err.Error(), emit)
	}
	args = cleanedArgs
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

	// Hard compaction mode comes before hooks and permissions: the engine
	// has asked for a compact long enough that only the context tool may
	// run, and the refusal itself carries the directive.
	if e.compactGate != nil {
		if directive := e.compactGate(call.Function.Name); directive != "" {
			return e.rejectResult(call, detail, directive, emit)
		}
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
	v := e.checkPlanGate(call, args)
	if v.Deny {
		return e.rejectResult(call, detail, v.Reason, emit)
	}
	planHint := ""
	if v.Miss {
		planHint = strings.TrimSpace(v.Reason + " " + v.Hint)
	}

	if msg, rejected := e.checkPermission(ctx, call, args, detail, emit); rejected {
		return msg
	}

	// A just-in-time step needs its own user approval after the permission
	// gate cleared: the plan's approval covers the contract, not the
	// irreversible effect this one step names.
	jitNotice := ""
	if v.JIT != nil {
		msg, notice, proceed := e.handoffJIT(ctx, call, args, detail, *v.JIT, emit)
		if !proceed {
			return msg
		}
		jitNotice = notice
	}

	// A call that cleared every gate settles its plan metadata before
	// dispatch: a piggybacked _plan envelope validates the tool's own
	// arguments against the schema the model saw, then completes the previous
	// step, swaps the working context and starts the named step as one atomic
	// plan write that survives the tool's runtime failure. Without an
	// envelope the call keeps the regular auto-start.
	if err := e.settleOrStart(ctx, call, tool, args, v, envelope); err != nil {
		return e.rejectResult(call, detail, err.Error(), emit)
	}

	// Starting this step may have fired inject_skill. New guidance refuses the
	// attempted working call before Run and returns the runtime-loaded bodies in
	// its result, so the model's retry is the first dispatch under that
	// guidance. A reminder of skills already in context is not worth a round:
	// it rides this call's own result below.
	var skillNote string
	if e.drainPlanSkills != nil {
		if preload, blocking := e.drainPlanSkills(); preload != "" {
			if blocking {
				state.skillRetryRequired = true
				reason := plangate.ReasonSkillPreload + "\n\n" + preload
				return e.rejectResult(call, detail, reason, emit)
			}
			skillNote = preload
		}
	}

	// The gate resolved and judged this call's paths before approval. The
	// guard rides the call so the module that performs the swap can ask the
	// same gate again with the write in flight: a directory swapped for a
	// symlink in the meantime fails closed instead of redirecting the file.
	runCtx := tools.WithMutationGuard(tools.WithToolCallID(ctx, call.ID), e.mutationGuard(call.Function.Name))
	result, err := tool.Run(runCtx, args)

	var (
		errText string
		content string
		output  string
	)
	if err != nil {
		if ctx.Err() != nil {
			// The cancellation is the answer; a recording failure under it
			// has nowhere honest to surface.
			_ = e.recordPlanAttempt(v, call, session.AttemptCanceled, "")
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

	// The documented PostTool contract: stop:true / exit 2 halts the agentic
	// loop. The stopped call's result still completes (pairing), and its
	// content tells the model why the run ended — a later turn reads it.
	if post.Stop {
		// The round runs no further call after this one, so this reason is
		// the one the run ends with; the guard only keeps it that way.
		if !state.stop.stopped {
			state.stop = hookStop{stopped: true, reason: post.Reason}
		}
		content = appendModelReminder(content, "A post-tool hook stopped the run: "+state.stop.Reason())
	}

	// Advice this very call parked (a plan compact action in its settle or
	// in the plan tool's Run) rides the call's own result — one boundary
	// earlier than the next-prompt drain, and drained here so that prompt
	// does not repeat it.
	if e.drainCompactAdvice != nil {
		if reminder := e.drainCompactAdvice(); reminder != "" {
			content = appendModelReminder(content, reminder)
		}
	}

	// A transition performed inside Run can also queue a step skill. The work
	// tool itself has already completed, so attach the loaded body to its result
	// for the next round; pre-dispatch handles ordinary auto-starts above.
	if e.drainPlanSkills != nil {
		if preload, _ := e.drainPlanSkills(); preload != "" {
			skillNote = strings.TrimSpace(skillNote + "\n\n" + preload)
		}
	}
	if skillNote != "" {
		content = appendModelReminder(content, skillNote)
	}

	// post.Stop is ignored until a later slice wires it into the agent loop.
	// The attempt summary is the bounded result the step keeps; the gate
	// notes below are guidance, not evidence, so they never enter it.
	summary := output
	if err != nil {
		summary = errText
	}
	if v.Note != "" {
		content = appendPlanGateHint(content, v.Note)
	}
	if planHint != "" {
		content = appendPlanGateHint(content, planHint)
	}
	if jitNotice != "" {
		content = appendPlanGateHint(content, jitNotice)
	}
	modelContent := appendHookContext(content, joinHookContexts(pre.Context, post.Context))

	if err != nil {
		delivered := emit(session.ToolData{Run: e.toolRun(call, session.ToolError, detail, errText, output)})
		modelContent = appendAttemptNotice(
			modelContent,
			e.recordPlanAttempt(v, call, attemptStatus(delivered, session.AttemptFailed), summary),
		)
		return e.toolMessage(call.ID, modelContent)
	}
	delivered := emit(session.ToolData{Run: e.toolRun(call, session.ToolDone, detail, "", output)})
	modelContent = appendAttemptNotice(
		modelContent,
		e.recordPlanAttempt(v, call, attemptStatus(delivered, session.AttemptSuccess), summary),
	)
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
		// The diff preview is computed lazily, only when a human is about
		// to look at the request: auto-approved calls never pay the file
		// I/O, and the gate has already decided without seeing it.
		req.Preview = writetool.AskPreview(ctx, call.Function.Name, args)
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

// checkPlanGate applies the approved-plan contract and returns the verdict.
// The verdict carries the pending step to auto-start and the note for a
// passing call; the caller derives the deny reason and the miss hint from it.
func (e *Executor) checkPlanGate(call llm.ToolCall, args json.RawMessage) plangate.Verdict {
	if e.planGate == nil || e.plan == nil {
		return plangate.Verdict{}
	}
	plan := e.plan()
	step := plangate.StepFromArgs(args)
	v := e.planGate.Check(plan, plangate.ToolCall{Name: call.Function.Name, Step: step})
	if v.Miss {
		if e.planGate.Recorder != nil {
			_ = e.planGate.Recorder.Record(e.missRecord(plan, call, step, v))
		}
		if e.planMiss != nil {
			// The miss counter is the gate's own verdict, so it counts even
			// when no miss recorder is wired: observability degrades last.
			e.planMiss()
		}
	}
	return v
}

// autoStart applies the verdict's pending-step start after every gate has
// cleared. A start that reports failure while the plan already shows the step
// in_progress lost a race with another call naming the same step and counts
// as success; any other failure fails the call closed.
func (e *Executor) autoStart(ctx context.Context, v plangate.Verdict) error {
	if !v.StartPending || v.StepID == "" || e.startStep == nil {
		return nil
	}
	if err := e.startStep(ctx, v.StepID); err != nil {
		ref := plangate.StepRef{ID: v.StepID}
		if item, ok := ref.Find(e.plan()); ok && item.Status == session.PlanInProgress {
			return nil
		}
		return fmt.Errorf("start plan step %q: %w", v.StepID, err)
	}
	return nil
}

// settleOrStart applies the call's plan metadata after every gate cleared.
// An envelope rides working calls only, needs the settle applier wired, and
// the tool's own arguments must validate against the schema the model saw —
// otherwise the call is rejected closed, with no plan mutation and no
// dispatch, so an invalid settle never lands half-applied. A plain call keeps
// the regular pending-step auto-start.
func (e *Executor) settleOrStart(
	ctx context.Context,
	call llm.ToolCall,
	tool tools.Tool,
	args json.RawMessage,
	v plangate.Verdict,
	envelope plangate.Envelope,
) error {
	if envelope.Empty() {
		return e.autoStart(ctx, v)
	}
	if plangate.IsExempt(call.Function.Name) {
		return fmt.Errorf("_plan rides working tool calls only, not %q", call.Function.Name)
	}
	if e.settlePlan == nil {
		return errors.New("plan settle metadata is not wired for this session; retry without _plan")
	}
	if err := tooldef.ValidateAgainstSchema(args, tool.Definition.Params); err != nil {
		return fmt.Errorf("_plan: invalid tool arguments: %w", err)
	}
	settle := session.PlanSettle{
		MutationID:     plangate.SettleMutationID(call.ID),
		Complete:       envelope.Complete,
		WorkingContext: envelope.WorkingContext,
	}
	if v.StartPending && v.StepID != "" {
		settle.StartStepID = v.StepID
	}
	if err := e.settlePlan(ctx, settle); err != nil {
		return fmt.Errorf("settle plan: %w", err)
	}
	return nil
}

// handoffJIT asks the user to approve a just-in-time step before dispatch.
// Approval records the durable grant and lets the call through; denial
// rejects the call with the step, action and risk the user saw. Without an
// ask handler the call fails closed. A grant that cannot be recorded does
// not undo the user's yes: the call runs and the model is told the approval
// was not durable, so the next call asks again.
func (e *Executor) handoffJIT(
	ctx context.Context,
	call llm.ToolCall,
	args json.RawMessage,
	detail string,
	demand plangate.JITDemand,
	emit func(session.ToolData) bool,
) (llm.Message, string, bool) {
	if e.ask == nil {
		reason := fmt.Sprintf(
			"plan step %q requires just-in-time approval but no ask handler is configured",
			demand.StepID,
		)
		return e.rejectResult(call, detail, reason, emit), "", false
	}
	// The overlay renders the request next to the question. The same
	// post-hook args the permission gate judged are the call the user is
	// approving; a synthesis that fails still shows the tool by name.
	req, err := permission.ExtractAt(call.Function.Name, args, e.cwd)
	if err != nil {
		req = permission.Request{Tool: call.Function.Name}
	}
	req.Preview = writetool.AskPreview(ctx, call.Function.Name, args)
	res, err := e.ask(ctx, req, demand.Question())
	if err != nil {
		reason := fmt.Sprintf("just-in-time approval failed: %v", err)
		return e.rejectResult(call, detail, reason, emit), "", false
	}
	if !res.Approved {
		return e.rejectResult(call, detail, demand.Rejected(res.Feedback), emit), "", false
	}
	notice := ""
	if e.approveStep != nil {
		if err := e.approveStep(demand.StepID, true); err != nil {
			notice = "just-in-time approval was not recorded: " + err.Error()
		}
	}
	return llm.Message{}, notice, true
}

// attemptStatus keeps the four attempt outcomes distinct: a result the event
// consumer never received is lost, whatever the tool itself reported.
func attemptStatus(delivered bool, terminal string) string {
	if !delivered {
		return session.AttemptLost
	}
	return terminal
}

// recordPlanAttempt files the call's bounded evidence on the step the verdict
// resolved. Recording never rewrites the tool's own result — the result
// already happened — but a failure is reported so the model learns its
// citation will not resolve. Steps without stable ids (legacy plans) keep no
// attempt history.
func (e *Executor) recordPlanAttempt(v plangate.Verdict, call llm.ToolCall, status, summary string) error {
	if e.recordStep == nil || v.StepID == "" {
		return nil
	}
	if err := e.recordStep(v.StepID, session.PlanAttempt{
		CallID:  call.ID,
		Tool:    call.Function.Name,
		Status:  status,
		Summary: summary,
		At:      time.Now(),
	}); err != nil {
		return fmt.Errorf("record attempt on step %q: %w", v.StepID, err)
	}
	return nil
}

// appendAttemptNotice surfaces an attempt-recording failure to the model
// without touching the tool result or the TUI.
func appendAttemptNotice(content string, err error) string {
	if err == nil {
		return content
	}
	return appendPlanGateHint(content, "attempt evidence was not recorded: "+err.Error())
}

func (e *Executor) missRecord(
	plan session.Plan,
	call llm.ToolCall,
	step plangate.StepRef,
	v plangate.Verdict,
) plangate.Miss {
	m := plangate.Miss{
		Session:      e.sessionID,
		Tool:         call.Function.Name,
		PlanStep:     step.Ordinal,
		StepID:       step.ID,
		PlanRevision: plan.Revision,
		Reason:       v.Reason,
		Phase:        string(e.planGate.Phase),
	}
	if item, ok := step.Find(plan); ok {
		m.StepStatus = string(item.Status)
		m.StepType = string(item.Type)
	}
	return m
}

// appendPlanGateHint adds model-facing guidance without touching TUI output.
func appendPlanGateHint(content, hint string) string {
	if content == "" {
		return "[plan gate] " + hint
	}
	return content + "\n\n[plan gate] " + hint
}

// appendModelReminder appends a self-contained wire-format reminder block
// to model-facing content without touching TUI output.
func appendModelReminder(content, reminder string) string {
	if content == "" {
		return reminder
	}
	return content + "\n\n" + reminder
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
