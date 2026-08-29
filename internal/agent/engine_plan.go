package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planLocked reads the current plan without taking engine.mu. The caller
// must hold the write lock (or own the engine exclusively during
// construction): rebind paths run under mu and must not re-enter it.
func (engine *Engine) planLocked() session.Plan {
	if engine.session == nil {
		return session.Plan{}
	}
	return engine.session.Plan()
}

func (engine *Engine) updatePlan(
	ctx context.Context,
	items []session.PlanItem,
) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	if err := engine.planRuntime.Current().ValidateItems(items); err != nil {
		return session.Plan{}, fmt.Errorf("agent: update plan: %w", err)
	}
	before := engine.Plan()
	autoApprove := engine.autoApproveNow()
	plan, err := engine.sessionRef().ReplacePlan(ctx, items, autoApprove)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: update plan: %w", err)
	}
	if fireErr := engine.fireAutoApprovalActions(before, plan); fireErr != nil {
		engine.publishPlan(plan)
		return plan, fmt.Errorf("agent: update plan: %w", fireErr)
	}
	engine.publishPlan(plan)
	return plan, nil
}

// createPlan stores a full v2 work contract as an unapproved draft. Unlike
// updatePlan it never consults the auto-approve policy: the contract is work
// the user has not seen yet, so approval stays the user's move. The returned
// diff names every material change against the previous snapshot, so a
// re-create after user feedback states exactly what moved.
func (engine *Engine) createPlan(
	ctx context.Context,
	contract session.PlanV2,
) (session.Plan, []session.PlanMaterialChange, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, nil, errors.New("agent: session unavailable")
	}
	if err := engine.planRuntime.Current().ValidateItems(contract.Items); err != nil {
		return session.Plan{}, nil, fmt.Errorf("agent: create plan: %w", err)
	}
	plan, diff, err := engine.sessionRef().ReplacePlanV2(ctx, contract, false)
	if err != nil {
		return session.Plan{}, nil, fmt.Errorf("agent: create plan: %w", err)
	}
	engine.publishPlan(plan)
	return plan, diff, nil
}

// autoApproveNow reads the policy under a read lock but invokes it outside:
// the callback may consult engine state and must never run under engine.mu.
func (engine *Engine) autoApproveNow() bool {
	engine.mu.RLock()
	fn := engine.autoApprove
	engine.mu.RUnlock()
	return fn != nil && fn()
}

// getPlan reads the current durable plan for the compact tool view.
func (engine *Engine) getPlan(context.Context) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return engine.sessionRef().Plan(), nil
}

// PatchPlan applies an atomic op batch through the same durable path as the
// other plan writers. Newly inserted steps are type-checked against the live
// plan policy, mirroring create and update; like updatePlan it consults the
// auto-approve policy, while the session alone decides whether the change
// was material enough to reset approval.
func (engine *Engine) PatchPlan(
	ctx context.Context,
	expectedRevision uint64,
	ops []session.PlanPatchOp,
) (session.Plan, session.PlanPatchSummary, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, session.PlanPatchSummary{}, errors.New("agent: session unavailable")
	}
	var inserted []session.PlanItem
	for _, op := range ops {
		if op.Op == session.PlanPatchInsertStep && op.Step != nil {
			inserted = append(inserted, *op.Step)
		}
	}
	if err := engine.planRuntime.Current().ValidateItems(inserted); err != nil {
		return session.Plan{}, session.PlanPatchSummary{}, fmt.Errorf("agent: patch plan: %w", err)
	}
	before := engine.Plan()
	autoApprove := engine.autoApproveNow()
	plan, summary, err := engine.sessionRef().PatchPlan(ctx, expectedRevision, ops, autoApprove)
	if err != nil {
		return session.Plan{}, session.PlanPatchSummary{}, fmt.Errorf("agent: patch plan: %w", err)
	}
	if fireErr := engine.fireAutoApprovalActions(before, plan); fireErr != nil {
		engine.publishPlan(plan)
		return plan, summary, fmt.Errorf("agent: patch plan: %w", fireErr)
	}
	engine.publishPlan(plan)
	return plan, summary, nil
}

// transitionPlan moves one step through the validated lifecycle state
// machine. It inserts no steps, so there is nothing to type-check: the
// session alone owns the matrix, the evidence contract, and the mutation
// ledger. Like PatchPlan it consults the auto-approve policy, and like every
// durable plan write it publishes only after the snapshot is on disk.
func (engine *Engine) transitionPlan(
	ctx context.Context,
	transition session.PlanTransition,
) (session.Plan, session.PlanTransitionResult, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, session.PlanTransitionResult{}, errors.New("agent: session unavailable")
	}
	if transition.Action == session.TransitionStart {
		// This door is the plan tool: a start here is a model call spent
		// purely on starting a step. The piggyback paths (auto-start, the
		// _plan envelope) apply starts without costing a call and never
		// come through here.
		engine.recordPlanStandaloneStart()
	}
	if err := engine.fireTransitionActions(ctx, transition); err != nil {
		return session.Plan{}, session.PlanTransitionResult{}, err
	}
	autoApprove := engine.autoApproveNow()
	plan, result, err := engine.sessionRef().TransitionPlan(ctx, transition, autoApprove)
	if err != nil {
		return session.Plan{}, session.PlanTransitionResult{}, fmt.Errorf("agent: transition plan: %w", err)
	}
	// A replay carries no new durable state, so the projection is already
	// current; publishing again would notify watchers of a non-event.
	if !result.Replayed {
		engine.publishPlan(plan)
	}
	return plan, result, nil
}

// autoStartStep is the harness-side half of "no separate start call": the
// executor invokes it after a gateable call cleared every gate and named a
// still-pending step. It applies the same audited start transition the plan
// tool offers, with a minted mutation id, and never touches approval — a
// start is operational, and the step being started is proof of active work.
func (engine *Engine) autoStartStep(ctx context.Context, stepID string) error {
	if engine == nil || engine.session == nil {
		return errors.New("agent: session unavailable")
	}
	plan := engine.Plan()
	if err := engine.fireStepStartEffects(ctx, plan, stepID); err != nil {
		return fmt.Errorf("agent: auto-start step: %w", err)
	}
	plan, result, err := engine.sessionRef().TransitionPlan(ctx, session.PlanTransition{
		Action:     session.TransitionStart,
		StepID:     stepID,
		MutationID: session.NewMutationID(),
	}, false)
	if err != nil {
		return fmt.Errorf("agent: auto-start step: %w", err)
	}
	if !result.Replayed {
		engine.publishPlan(plan)
	}
	return nil
}

// settlePlanFromCall is the engine half of the _plan envelope: the executor
// hands it one settle the session validates and applies as a single durable,
// idempotent write, and the next inference sees the fresh projection. A
// replayed settle changed nothing, so watchers are not woken for a non-event.
func (engine *Engine) settlePlanFromCall(ctx context.Context, settle session.PlanSettle) error {
	if engine == nil || engine.session == nil {
		return errors.New("agent: session unavailable")
	}
	if err := engine.fireSettleActions(ctx, settle); err != nil {
		return fmt.Errorf("agent: settle plan from call: %w", err)
	}
	plan, result, err := engine.sessionRef().SettlePlanFromCall(settle)
	if err != nil {
		return fmt.Errorf("agent: settle plan from call: %w", err)
	}
	if !result.Replayed {
		engine.publishPlan(plan)
	}
	return nil
}

// recordStepAttempt is the harness-side half of attempt evidence: the
// executor files one bounded record per accepted gateable call, and the write
// is durable and published exactly like any other plan change.
func (engine *Engine) recordStepAttempt(stepID string, attempt session.PlanAttempt) error {
	if engine == nil || engine.session == nil {
		return errors.New("agent: session unavailable")
	}
	plan, err := engine.sessionRef().RecordPlanAttempt(stepID, attempt)
	if err != nil {
		return fmt.Errorf("agent: record plan attempt: %w", err)
	}
	engine.publishPlan(plan)
	return nil
}

// publishPlan refreshes the inference-facing projection after a durable plan
// write: the next round must see fresh bounded metadata, and watchers hear
// about the new snapshot only after it is durable.
func (engine *Engine) publishPlan(plan session.Plan) {
	// Rebuilding only the client leaves the executing tool registry untouched.
	engine.mu.Lock()
	engine.rebindClient(engine.buildToolList())
	engine.mu.Unlock()
	if engine.onPlanUpdated != nil {
		engine.onPlanUpdated(plan.Clone())
	}
}

// SetAutoApprove binds the auto-approve policy consulted by updatePlan, so a
// model-edited plan is approved synchronously when auto-approval is enabled.
func (engine *Engine) SetAutoApprove(fn func() bool) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.autoApprove = fn
}

// Plan returns the latest durable model-managed plan snapshot.
func (engine *Engine) Plan() session.Plan {
	sess := engine.sessionRef()
	if engine == nil || sess == nil {
		return session.Plan{}
	}
	return sess.Plan()
}

// PlanRuntime returns the shared live plan policy source.
func (engine *Engine) PlanRuntime() *plangate.Runtime {
	if engine == nil {
		return nil
	}
	return engine.planRuntime
}

// RenamePlanStepTypes migrates current-plan references for a global settings
// rename while preserving approval. The caller publishes the matching policy.
func (engine *Engine) RenamePlanStepTypes(
	ctx context.Context,
	renames map[session.StepType]session.StepType,
) (session.Plan, error) {
	if engine == nil || engine.sessionRef() == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	plan, err := engine.sessionRef().RenamePlanStepTypes(ctx, renames)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: rename plan step types: %w", err)
	}
	if engine.onPlanUpdated != nil {
		engine.onPlanUpdated(plan.Clone())
	}
	return plan, nil
}

// SetPlanGate replaces the plan gate and rebinds the executor so the new
// phase takes effect on the next tool round. nil disables plan gating.
func (engine *Engine) SetPlanGate(gate *plangate.Checker) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.planGate = gate
	if engine.client != nil {
		engine.rebindTools()
	}
}

// SetPlanEnabled turns the plan feature on or off live: the plan tool, the
// plan-gate prompt block and hint, plan_step injection and the gate's tool
// filtering. The durable plan, runtime and gate checker survive untouched, so
// re-enabling restores the previous state; engines without a plan runtime
// (sub-agents) ignore the call. Safe mid-round: the swap happens under mu and
// a running round finishes on the executor and client it started with.
func (engine *Engine) SetPlanEnabled(enabled bool) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.planEnabled == enabled || (enabled && engine.planRuntime == nil) {
		return
	}
	engine.planEnabled = enabled
	if !enabled && engine.mode == ModePlan {
		// Plan mode exists to draft plans; without the feature it would keep
		// the read-only toolset and the plan prompt appendix for nothing.
		engine.mode = ModeUsePlan
	}
	engine.rebindTools()
}

// SetPlanApproved flips the user-owned approval flag durably and rebinds so
// the next inference round sees the new gate posture and hint.
func (engine *Engine) SetPlanApproved(approved bool) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	if approved {
		if err := engine.firePlanApprovalActions(); err != nil {
			return session.Plan{}, fmt.Errorf("agent: set plan approved: %w", err)
		}
	}
	plan, err := engine.sessionRef().SetPlanApproved(approved)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: set plan approved: %w", err)
	}
	engine.publishPlan(plan)
	return plan, nil
}

// SetStepJITApproved records or withdraws the user-owned just-in-time
// approval for one plan step and republishes the snapshot. It is the durable
// half of the executor's approval handoff, exposed for user-owned surfaces.
func (engine *Engine) SetStepJITApproved(stepID string, granted bool) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	plan, err := engine.sessionRef().SetStepJITApproved(stepID, granted)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: set step just-in-time approval: %w", err)
	}
	engine.publishPlan(plan)
	return plan, nil
}

// approveStepJIT adapts SetStepJITApproved to the executor's grant callback.
func (engine *Engine) approveStepJIT(stepID string, granted bool) error {
	_, err := engine.SetStepJITApproved(stepID, granted)
	return err
}

// ClearPlan drops the durable plan, resets its revision counter, and republishes
// the empty snapshot so the sidebar reacts to the reset.
func (engine *Engine) ClearPlan() (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	plan, err := engine.sessionRef().ClearPlan()
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: clear plan: %w", err)
	}
	engine.publishPlan(plan)
	return plan, nil
}

func (engine *Engine) syncPlanProjection() {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.planEnabled && engine.planRuntime.Current() != engine.projectedPlanPolicy {
		engine.rebindTools()
	}
}

// inferenceContext projects durable session history into one provider request.
// The current plan never joins the messages: it reaches the model through the
// system prompt only (gate block and hint), so providers see exactly the
// durable history and nothing synthetic.
func (*Engine) inferenceContext(sess *Session) []llm.Message {
	return slices.Clone(sess.BuildContext())
}

// applyPlanGatePhase keeps the gate's enforcement phase in lockstep with the
// turn posture: UsePlan denies misses, while Build and Plan only hint. The
// checker is replaced copy-on-write — an executor mid-round keeps the one it
// was bound with and finishes under the phase it started with.
// The caller must hold engine.mu.
func (engine *Engine) applyPlanGatePhase() {
	if engine.planGate == nil {
		return
	}
	next := plangate.PhaseHint
	if engine.mode == ModeUsePlan {
		next = plangate.PhaseDeny
	}
	c := *engine.planGate
	c.Phase = next
	engine.planGate = &c
}
