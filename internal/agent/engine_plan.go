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
	autoApprove := engine.autoApproveNow()
	plan, err := engine.sessionRef().ReplacePlan(ctx, items, autoApprove)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: update plan: %w", err)
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

// patchPlan applies an atomic op batch through the same durable path as the
// other plan writers. Newly inserted steps are type-checked against the live
// plan policy, mirroring create and update; like updatePlan it consults the
// auto-approve policy, while the session alone decides whether the change
// was material enough to reset approval.
func (engine *Engine) patchPlan(
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
	autoApprove := engine.autoApproveNow()
	plan, summary, err := engine.sessionRef().PatchPlan(ctx, expectedRevision, ops, autoApprove)
	if err != nil {
		return session.Plan{}, session.PlanPatchSummary{}, fmt.Errorf("agent: patch plan: %w", err)
	}
	engine.publishPlan(plan)
	return plan, summary, nil
}

// transitionPlan moves one step through the validated lifecycle state
// machine. It inserts no steps, so there is nothing to type-check: the
// session alone owns the matrix, the evidence contract, and the mutation
// ledger. Like patchPlan it consults the auto-approve policy, and like every
// durable plan write it publishes only after the snapshot is on disk.
func (engine *Engine) transitionPlan(
	ctx context.Context,
	transition session.PlanTransition,
) (session.Plan, session.PlanTransitionResult, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, session.PlanTransitionResult{}, errors.New("agent: session unavailable")
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

// SetPlanApproved flips the user-owned approval flag durably and rebinds so
// the next inference round sees the new gate posture and hint.
func (engine *Engine) SetPlanApproved(approved bool) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	plan, err := engine.sessionRef().SetPlanApproved(approved)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: set plan approved: %w", err)
	}
	engine.publishPlan(plan)
	return plan, nil
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
// The current plan is intentionally transient: every inference sees one fresh
// authoritative snapshot while the append-only session never stores copies.
func (engine *Engine) inferenceContext(sess *Session) []llm.Message {
	messages := slices.Clone(sess.BuildContext())
	if !engine.planEnabled {
		return messages
	}
	// The plan snapshot is presented as the output of a tool round (an
	// assistant tool_call plus its tool result) rather than as a user
	// utterance: providers treat RoleTool as the result of a prior call, so the
	// model reads the plan as harness data it already asked for, not as a fresh
	// user message it must answer or restate.
	callID := "plan_snapshot"
	return append(messages,
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: callID, Type: "function", Function: llm.Function{Name: "plan", Arguments: "{}"}},
			},
		},
		llm.Message{Role: llm.RoleTool, ToolCallID: callID, Content: plangate.PromptSnapshot(engine.Plan())},
	)
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
