package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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

// executePlanAction runs one built-in. compact reuses the /compact engine:
// synchronous, durable, its UI events forwarded through the session event
// sink. inject_skill parks the names for the next composed prompt — the
// step's first turn after the event reads those skills, bodies load lazily.
func (engine *Engine) executePlanAction(ctx context.Context, action session.PlanAction) error {
	switch action.Type {
	case session.PlanActionCompact:
		return engine.CompactNow(ctx, func(ev session.Event) bool {
			engine.emitSessionEvent(ev)
			return true
		})
	case session.PlanActionInjectSkill:
		engine.queuePlanSkills(action.Skills)
		return nil
	default:
		// Authoring validation keeps unknown types out of the plan; a stale
		// snapshot from an older schema is refused loudly, not skipped.
		return fmt.Errorf("unknown plan action type %q", action.Type)
	}
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
		return engine.runPlanActions(
			ctx, plan, planActionsForEvent(plan, transition.StepID, session.PlanActionOnStepStart), false,
		)
	case session.TransitionComplete:
		batch := planActionsForEvent(plan, transition.StepID, session.PlanActionOnStepEnd)
		if transition.PlanResult != "" {
			batch = append(batch, planActionsForEvent(plan, "", session.PlanActionOnPlanEnd)...)
		}
		return engine.runPlanActions(ctx, plan, batch, false)
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
	var batch []planActionInvocation
	if settle.Complete != nil {
		batch = append(batch, planActionsForEvent(plan, settle.Complete.StepID, session.PlanActionOnStepEnd)...)
		if settle.Complete.PlanResult != "" {
			batch = append(batch, planActionsForEvent(plan, "", session.PlanActionOnPlanEnd)...)
		}
	}
	if settle.StartStepID != "" && stepIsPending(plan, settle.StartStepID) {
		batch = append(batch, planActionsForEvent(plan, settle.StartStepID, session.PlanActionOnStepStart)...)
	}
	return engine.runPlanActions(ctx, plan, batch, false)
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

// queuePlanSkills parks skill names an inject_skill action produced. The
// next composed user prompt consumes them, so the step's first turn after
// the event is told to read those skills.
func (engine *Engine) queuePlanSkills(names []string) {
	if len(names) == 0 {
		return
	}
	engine.mu.Lock()
	engine.planSkills = append(engine.planSkills, names...)
	engine.mu.Unlock()
}

// mergePlanSkills drains the parked queue into the caller's own selection;
// plan-injected skills ride exactly one prompt.
func (engine *Engine) mergePlanSkills(selected []string) []string {
	engine.mu.Lock()
	queued := engine.planSkills
	engine.planSkills = nil
	engine.mu.Unlock()
	if len(queued) == 0 {
		return selected
	}
	return slices.Compact(append(queued, selected...))
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
