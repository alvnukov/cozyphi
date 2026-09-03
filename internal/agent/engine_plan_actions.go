package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planActionInvocation addresses one action definition in the live plan: the
// step it lives on (empty id = the plan-level list) and its index there.
type planActionInvocation struct {
	stepID string
	index  int
	action session.PlanAction
}

// planActionsForEvent gathers the actions one lifecycle event will fire from
// a durable snapshot: the step's own list for step events, the plan's list
// for plan events. The caller decides whether the event fires at all.
func planActionsForEvent(plan session.Plan, stepID string, event session.PlanActionEvent) []planActionInvocation {
	var invocations []planActionInvocation
	collect := func(actions []session.PlanAction) {
		for i, action := range actions {
			if action.Event == event {
				invocations = append(invocations, planActionInvocation{stepID: stepID, index: i, action: action})
			}
		}
	}
	if event == session.PlanActionOnPlanStart || event == session.PlanActionOnPlanEnd {
		collect(plan.Actions)
		return invocations
	}
	for _, item := range plan.Items {
		if item.ID == stepID {
			collect(item.Actions)
			break
		}
	}
	return invocations
}

// stepIsPending reports whether the step can still be started, so a settle
// that lost the start race to another call fires no step_start actions for
// the winner.
func stepIsPending(plan session.Plan, stepID string) bool {
	for _, item := range plan.Items {
		if item.ID == stepID {
			return item.Status == session.PlanPending
		}
	}
	return false
}

// runPlanActions executes one event's action batch synchronously, before the
// transition's durable write. Every outcome is recorded where the action
// lives and announced as a PlanActionRan event; the first failure refuses the
// batch, so the plan never advances past automation that did not happen.
// Automation exists only in approved plans — the approval door is the one
// caller allowed to pass approvedNow, for its own plan_start batch.
func (engine *Engine) runPlanActions(
	ctx context.Context,
	plan session.Plan,
	batch []planActionInvocation,
	approvedNow bool,
) error {
	if len(batch) == 0 || (!plan.Approved && !approvedNow) {
		return nil
	}
	for _, inv := range batch {
		run := session.PlanActionRun{Status: session.PlanActionRunOK, At: time.Now()}
		actionErr := engine.executePlanAction(ctx, inv.action)
		if actionErr != nil {
			run.Status = session.PlanActionRunFailed
			run.Error = actionErr.Error()
		}
		if _, recordErr := engine.sessionRef().AppendPlanActionRun(inv.stepID, inv.index, run); recordErr != nil {
			// A run the plan cannot record is a run that did not verifiably
			// happen: fail the action rather than move on without evidence.
			actionErr = errors.Join(actionErr, recordErr)
		}
		engine.emitSessionEvent(session.PlanActionRan{
			StepID: inv.stepID,
			Event:  inv.action.Event,
			Type:   inv.action.Type,
			Status: run.Status,
			Error:  run.Error,
		})
		if actionErr != nil {
			return fmt.Errorf("agent: plan action %s on %s %s failed: %w",
				inv.action.Type, inv.action.Event, actionWhere(inv.stepID), actionErr)
		}
	}
	return nil
}

func actionWhere(stepID string) string {
	if stepID == "" {
		return "plan"
	}
	return "step " + stepID
}

// executePlanAction runs one built-in. compact queues a compaction
// recommendation for the next composed prompt — the model records what must
// survive and calls the compact itself at a moment it picks, instead of a
// synchronous compaction interrupting the step. inject_skill loads and parks
// complete skill bodies for the step's first model boundary.
func (engine *Engine) executePlanAction(_ context.Context, action session.PlanAction) error {
	switch action.Type {
	case session.PlanActionCompact:
		// No CompactNow: the action marks the boundary, the model runs the
		// compact (see compact_advice.go). Queueing cannot fail, so the
		// transition is never refused by this action.
		engine.queuePlanCompactAdvice()
		return nil
	case session.PlanActionInjectSkill:
		// The user's off marks (DisabledSkills) ride the action: injection
		// honors them — an empty effective list injects nothing, quietly.
		engine.queuePlanSkills(action.EffectiveSkills())
		return nil
	default:
		// Authoring validation keeps unknown types out of the plan; a stale
		// snapshot from an older schema is refused loudly, not skipped.
		return fmt.Errorf("unknown plan action type %q", action.Type)
	}
}

// fireStepStartEffects is the one ordering every step-start door shares:
// resolve the pinned model first — an unrunnable pin refuses the move before
// any action spends work — then the step's actions, then the model switch.
// Draft plans stay passive: no model moves before approval.
// The batch runs ahead of the durable transition write on purpose — a step
// never advances past unrun automation — so a write that fails and is retried
// re-runs the batch; compact and inject_skill are safe to fire twice.
func (engine *Engine) fireStepStartEffects(ctx context.Context, plan session.Plan, stepID string) error {
	target, pinned, err := engine.resolveStepModel(stepID, planStepModelName(plan, stepID))
	if err != nil {
		return err
	}
	if err := engine.runPlanActions(
		ctx, plan, planActionsForEvent(plan, stepID, session.PlanActionOnStepStart), false,
	); err != nil {
		return err
	}
	if !plan.Approved {
		return nil
	}
	return engine.switchStepModel(target, pinned)
}

// unstartedAutomationError refuses a completion that would skip start
// automation a step never fired. A pending step still owes its model pin and
// step_start actions; completing it from pending would record the work as
// done over promises the plan made — the silent skip the contract forbids.
// nil when the step owes nothing, so plain pending steps keep the one-call
// completion door.
func unstartedAutomationError(plan session.Plan, stepID string) error {
	if !stepIsPending(plan, stepID) {
		return nil
	}
	owed := ""
	if planStepModelName(plan, stepID) != "" {
		owed = "a model pin"
	}
	if len(planActionsForEvent(plan, stepID, session.PlanActionOnStepStart)) > 0 {
		if owed != "" {
			owed += " and "
		}
		owed += "step_start actions"
	}
	if owed == "" {
		return nil
	}
	return fmt.Errorf(
		"plan step %q is still pending and owes %s that never ran; "+
			"start the step — pass plan_step on its next working call — then complete it",
		stepID, owed,
	)
}

// fireTransitionActions runs the automation one plan-tool transition will
// fire, before its durable write: step_start on start; step_end plus the
// plan_end batch on a closing complete. Block, resume, cancel and reopen
// stay non-automatic, and a mutation the ledger already recorded replays
// without re-running anything.
func (engine *Engine) fireTransitionActions(ctx context.Context, transition session.PlanTransition) error {
	if engine.sessionRef().HasPlanMutation(transition.MutationID) {
		return nil
	}
	plan := engine.Plan()
	switch transition.Action {
	case session.TransitionStart:
		return engine.fireStepStartEffects(ctx, plan, transition.StepID)
	case session.TransitionComplete:
		if err := unstartedAutomationError(plan, transition.StepID); err != nil {
			return err
		}
		batch := planActionsForEvent(plan, transition.StepID, session.PlanActionOnStepEnd)
		if transition.PlanResult != "" {
			batch = append(batch, planActionsForEvent(plan, "", session.PlanActionOnPlanEnd)...)
		}
		if err := engine.runPlanActions(ctx, plan, batch, false); err != nil {
			return err
		}
		if transition.PlanResult != "" {
			engine.restoreSessionModelOnClose()
		}
		return nil
	}
	return nil
}

// fireSettleActions mirrors fireTransitionActions for the _plan envelope:
// the settle's completing step fires step_end (plus plan_end when it carries
// a plan result) and its target step fires step_start — one all-or-nothing
// batch ahead of the single durable write. A settle whose mutation the
// ledger already recorded replays without side effects.
func (engine *Engine) fireSettleActions(ctx context.Context, settle session.PlanSettle) error {
	if engine.sessionRef().HasPlanMutation(settle.MutationID) {
		return nil
	}
	plan := engine.Plan()
	if settle.Complete != nil {
		if err := unstartedAutomationError(plan, settle.Complete.StepID); err != nil {
			return err
		}
	}
	// The started step's model resolves before any action runs: an
	// unrunnable pin refuses the whole settle with nothing spent.
	starting := settle.StartStepID != "" && stepIsPending(plan, settle.StartStepID)
	var target llm.ModelConfig
	var pinned bool
	if starting {
		var err error
		target, pinned, err = engine.resolveStepModel(settle.StartStepID, planStepModelName(plan, settle.StartStepID))
		if err != nil {
			return err
		}
	}
	var batch []planActionInvocation
	if settle.Complete != nil {
		batch = append(batch, planActionsForEvent(plan, settle.Complete.StepID, session.PlanActionOnStepEnd)...)
		if settle.Complete.PlanResult != "" {
			batch = append(batch, planActionsForEvent(plan, "", session.PlanActionOnPlanEnd)...)
		}
	}
	if starting {
		batch = append(batch, planActionsForEvent(plan, settle.StartStepID, session.PlanActionOnStepStart)...)
	}
	if err := engine.runPlanActions(ctx, plan, batch, false); err != nil {
		return err
	}
	if settle.Complete != nil && settle.Complete.PlanResult != "" {
		engine.restoreSessionModelOnClose()
	}
	if starting && plan.Approved {
		return engine.switchStepModel(target, pinned)
	}
	return nil
}

// firePlanApprovalActions runs the plan_start batch ahead of the approval
// write: approval is the transition, so a failed action refuses it. Only the
// unapproved-to-approved move fires — re-approving an approved plan is a
// no-op for automation. The approval door lives in the TUI and carries no
// request context; the compaction it may run is bounded by its own HTTP
// deadlines.
func (engine *Engine) firePlanApprovalActions() error {
	plan := engine.Plan()
	if plan.Approved {
		return nil
	}
	return engine.runPlanActions(
		context.Background(), plan,
		planActionsForEvent(plan, "", session.PlanActionOnPlanStart), true,
	)
}

// fireAutoApprovalActions runs the plan_start batch after a writer that
// carried the auto-approval policy flipped the plan unapproved→approved:
// commitPlanLocked stamps approval inside the durable write, so unlike the
// TUI approval door the batch cannot refuse it — a failure records its run
// and surfaces the error with the approval standing.
func (engine *Engine) fireAutoApprovalActions(before, after session.Plan) error {
	if before.Approved || !after.Approved {
		return nil
	}
	return engine.runPlanActions(
		context.Background(), after,
		planActionsForEvent(after, "", session.PlanActionOnPlanStart), false,
	)
}

type planSkillPreload struct {
	name string
	body string
}

// queuePlanSkills loads complete bodies when the action fires. Loading before a
// possible step-model switch keeps the selected catalog stable; the queued
// plain text then needs no model-issued read call. A name the catalog cannot
// supply is queued with an empty body and falls back to the read instruction
// at drain time.
func (engine *Engine) queuePlanSkills(names []string) {
	if len(names) == 0 {
		return
	}
	catalog, err := skills.LoadSkills(engine.skillPath)
	if err != nil {
		debuglog.Logf("plan: load skills for preload from %s: %v", engine.skillPath, err)
	}
	queued := make([]planSkillPreload, 0, len(names))
	for _, name := range names {
		preload := planSkillPreload{name: name}
		if skill := skills.Find(catalog, name); skill != nil && skill.Body != "" {
			preload.name = skill.Name
			preload.body = skill.Body
		} else {
			debuglog.Logf("plan: skill %q has no body to preload; falling back to a read instruction", name)
		}
		queued = append(queued, preload)
	}
	engine.mu.Lock()
	engine.planSkills = append(engine.planSkills, queued...)
	engine.mu.Unlock()
}

// drainPlanSkills renders queued bodies in action order, dropping duplicate
// names while retaining the first selection. The result is ordinary text, not
// read-tool/hashline output. A skill whose body never loaded is not silently
// announced: it falls back to the instruction that sends the model to its
// SKILL.md, so a step is never told to follow guidance it cannot see. A body
// already delivered in this session is named, not repeated — the same skill on
// five steps costs one copy, and compaction rearms it.
//
// blocking reports whether the text carries guidance the model has not seen:
// only then is it worth refusing the call that started the step. A pure
// reminder rides the result of the work it accompanies.
func (engine *Engine) drainPlanSkills() (text string, blocking bool) {
	engine.mu.Lock()
	queued := engine.planSkills
	engine.planSkills = nil
	delivered := make(map[string]struct{}, len(engine.planSkillsDelivered))
	maps.Copy(delivered, engine.planSkillsDelivered)
	engine.mu.Unlock()
	if len(queued) == 0 {
		return "", false
	}

	var (
		out      strings.Builder
		missing  []string
		repeated []string
		fresh    []string
		seen     = make(map[string]struct{}, len(queued))
	)
	for _, skill := range queued {
		key := strings.ToLower(strings.TrimSpace(skill.name))
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if skill.body == "" {
			missing = append(missing, skill.name)
			continue
		}
		if _, sent := delivered[key]; sent {
			repeated = append(repeated, skill.name)
			continue
		}
		fresh = append(fresh, key)
		if out.Len() == 0 {
			out.WriteString(
				"The runtime preloaded these plan-step skills. Follow them for this step; their SKILL.md needs no read call.",
			)
		}
		out.WriteString("\n\n## Skill: ")
		out.WriteString(skill.name)
		out.WriteString("\n\n")
		out.WriteString(skill.body)
	}
	if instruction := pendingSkillsInstruction(engine.skillPath, missing); instruction != "" {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(instruction)
	}
	blocking = out.Len() > 0
	if len(repeated) > 0 {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString("Already preloaded earlier in this session and still in force for this step: ")
		out.WriteString(strings.Join(repeated, ", "))
		out.WriteString(".")
	}
	if len(fresh) > 0 {
		engine.mu.Lock()
		if engine.planSkillsDelivered == nil {
			engine.planSkillsDelivered = make(map[string]struct{}, len(fresh))
		}
		for _, key := range fresh {
			engine.planSkillsDelivered[key] = struct{}{}
		}
		engine.mu.Unlock()
	}
	return out.String(), blocking
}

// forgetDeliveredPlanSkills drops the record of which skill bodies are in
// context. Compaction may have summarized them away, so the next step that
// names one must get the body again rather than a reminder of text that is
// no longer there.
func (engine *Engine) forgetDeliveredPlanSkills() {
	engine.mu.Lock()
	engine.planSkillsDelivered = nil
	engine.mu.Unlock()
}

// emitSessionEvent forwards an event the engine produces outside a streaming
// round — plan action runs today — to the wired sink. A compact action's
// compaction events reach the UI on the same road the /compact path uses.
func (engine *Engine) emitSessionEvent(ev session.Event) {
	engine.mu.RLock()
	sink := engine.sessionEvents
	engine.mu.RUnlock()
	if sink != nil {
		sink(ev)
	}
}
