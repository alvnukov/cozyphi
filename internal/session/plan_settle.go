package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// SettleAction names the piggybacked settle in the mutation ledger so it
// never collides with a step transition action.
const SettleAction = "settle"

// PlanSettle is the plan metadata one working tool call piggybacks through
// the _plan envelope: settle the previous step, replace the working context,
// and start the target step. Every part is optional on its own, but a settle
// that carries no work is refused — metadata that moves nothing only spends
// tokens.
type PlanSettle struct {
	// MutationID is the idempotency key; the executor derives it from the
	// tool call id so a retried or reconciled call replays the recorded
	// result instead of applying the settle twice.
	MutationID string `json:"mutationId"`
	// Complete settles one previous step; nil leaves steps untouched.
	Complete *PlanTransition `json:"complete,omitempty"`
	// WorkingContext replaces the plan's working context; nil keeps it.
	WorkingContext *string `json:"workingContext,omitempty"`
	// StartStepID starts one still-pending target step; empty leaves starts
	// to the executor's regular auto-start.
	StartStepID string `json:"startStepId,omitempty"`
}

// PlanSettleResult is the compact answer to one settle: which parts applied,
// and the single revision they all share.
type PlanSettleResult struct {
	Replayed       bool       `json:"replayed,omitempty"`
	Completed      bool       `json:"completed,omitempty"`
	ContextUpdated bool       `json:"contextUpdated,omitempty"`
	Started        bool       `json:"started,omitempty"`
	StepID         string     `json:"stepId,omitempty"`
	From           PlanStatus `json:"from,omitempty"`
	To             PlanStatus `json:"to,omitempty"`
	Revision       uint64     `json:"revision"`
	EventIDs       []string   `json:"eventIds,omitempty"`
	// Closed names the plan-level result this settle also recorded; empty
	// when the complete closed a step only.
	Closed PlanResult `json:"closed,omitempty"`
}

// SettlePlanFromCall applies one piggybacked settle as a single durable,
// idempotent write: completing the previous step, swapping the working
// context and starting the target step share one revision, one commit and
// one audit trail, so there is no mid-settle state and a retry replays the
// recorded result. Validation reuses the transition machinery — the same
// state machine, the same evidence contract — and any invalid part refuses
// the whole settle with no partial mutation. A start whose target is already
// in_progress counts as success: another call won that race, and this
// settle's own work still stands.
func (sm *Manager) SettlePlanFromCall(settle PlanSettle) (Plan, PlanSettleResult, error) {
	if sm == nil {
		return Plan{}, PlanSettleResult{}, errors.New("session: plan manager is nil")
	}
	if settle.Complete == nil && settle.WorkingContext == nil && strings.TrimSpace(settle.StartStepID) == "" {
		return Plan{}, PlanSettleResult{}, errors.New("session: settle carries no work")
	}
	if err := validateMutationID(settle.MutationID); err != nil {
		return Plan{}, PlanSettleResult{}, fmt.Errorf("session: settle: %w", err)
	}
	if settle.WorkingContext != nil && utf8.RuneCountInString(*settle.WorkingContext) > maxPlanWorkingContextRunes {
		return Plan{}, PlanSettleResult{}, fmt.Errorf(
			"session: settle working context exceeds %d characters", maxPlanWorkingContextRunes,
		)
	}
	// The envelope contract allows exactly one transition action: completing
	// a step. Everything else the plan tool offers stays a plan-tool call.
	if settle.Complete != nil {
		settle.Complete.Action = TransitionComplete
		normalizeTransition(settle.Complete)
	}
	settle.StartStepID = strings.TrimSpace(settle.StartStepID)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.plan.Schema != PlanSchemaV2 {
		return Plan{}, PlanSettleResult{}, errors.New("session: settle requires a v2 plan")
	}

	completedStep := ""
	if settle.Complete != nil {
		completedStep = settle.Complete.StepID
	}
	if recorded, found := findPlanMutation(sm.plan.Mutations, settle.MutationID); found {
		if recorded.Result.Action != SettleAction || recorded.Result.StepID != completedStep {
			return Plan{}, PlanSettleResult{}, fmt.Errorf(
				"session: mutation id %q was already used for %s step %q",
				settle.MutationID, recorded.Result.Action, recorded.Result.StepID,
			)
		}
		replayed := PlanSettleResult{
			Replayed: true,
			StepID:   recorded.Result.StepID,
			From:     recorded.Result.From,
			To:       recorded.Result.To,
			Revision: recorded.Result.Revision,
		}
		if recorded.Settle != nil {
			replayed = *recorded.Settle
			replayed.Replayed = true
		}
		return sm.plan.Clone(), replayed, nil
	}

	candidate := sm.plan.Clone()
	result := PlanSettleResult{}
	var eventIDs []string

	if settle.Complete != nil {
		idx := findStepByID(candidate.Items, settle.Complete.StepID)
		if idx < 0 {
			return Plan{}, PlanSettleResult{}, fmt.Errorf(
				"session: step %q not found", settle.Complete.StepID,
			)
		}
		from := candidate.Items[idx].Status
		spec := planTransitions[TransitionComplete]
		if !slices.Contains(spec.from, from) {
			return Plan{}, PlanSettleResult{}, fmt.Errorf(
				"session: cannot complete step %q from %s; allowed actions: %s",
				settle.Complete.StepID, from, allowedTransitionsFrom(from),
			)
		}
		if err := validateTransitionPayload(*settle.Complete); err != nil {
			return Plan{}, PlanSettleResult{}, err
		}
		if err := validateCompleteAttemptRefs(*settle.Complete, sm.plan.Items[idx]); err != nil {
			return Plan{}, PlanSettleResult{}, err
		}
		applyTransition(&candidate.Items[idx], spec, *settle.Complete)
		var finishEvent *PlanEvent
		if settle.Complete.PlanResult != "" {
			var err error
			finishEvent, err = finishCandidatePlan(
				&candidate, settle.Complete.PlanResult, settle.MutationID, sm.generateID(),
			)
			if err != nil {
				return Plan{}, PlanSettleResult{}, fmt.Errorf(
					"session: settle complete step %q: %w", settle.Complete.StepID, err,
				)
			}
		}
		event := PlanEvent{
			ID:       sm.generateID(),
			At:       time.Now(),
			Mutation: settle.MutationID,
			Action:   TransitionComplete,
			StepID:   settle.Complete.StepID,
			From:     from,
			To:       spec.to,

			Outcome:          settle.Complete.Outcome,
			Evidence:         settle.Complete.Evidence,
			EvidenceRefs:     settle.Complete.EvidenceRefs,
			NoEvidenceReason: settle.Complete.NoEvidenceReason,
		}
		candidate.Events = appendBoundedTail(candidate.Events, event)
		if finishEvent != nil {
			candidate.Events = appendBoundedTail(candidate.Events, *finishEvent)
		}
		result.Completed = true
		result.StepID = settle.Complete.StepID
		result.From = from
		result.To = spec.to
		// The candidate carries the finish when one applied; the settle
		// receipt then reports the close alongside the completed step.
		result.Closed = candidate.Result
		eventIDs = append(eventIDs, event.ID)
	}

	if settle.WorkingContext != nil {
		candidate.WorkingContext = *settle.WorkingContext
		result.ContextUpdated = true
	}

	if settle.StartStepID != "" {
		idx := findStepByID(candidate.Items, settle.StartStepID)
		if idx < 0 {
			return Plan{}, PlanSettleResult{}, fmt.Errorf(
				"session: step %q not found", settle.StartStepID,
			)
		}
		switch candidate.Items[idx].Status {
		case PlanInProgress:
			// Another call already started the target. The settle's own work
			// still stands; a duplicate start event would lie about it.
		case PlanPending:
			event := PlanEvent{
				ID:       sm.generateID(),
				At:       time.Now(),
				Mutation: settle.MutationID,
				Action:   TransitionStart,
				StepID:   settle.StartStepID,
				From:     PlanPending,
				To:       PlanInProgress,
			}
			candidate.Items[idx].Status = PlanInProgress
			candidate.Events = appendBoundedTail(candidate.Events, event)
			result.Started = true
			eventIDs = append(eventIDs, event.ID)
		default:
			return Plan{}, PlanSettleResult{}, fmt.Errorf(
				"session: settle cannot start step %q from %s",
				settle.StartStepID, candidate.Items[idx].Status,
			)
		}
	}

	checked, err := revalidatePatchedPlan(candidate)
	if err != nil {
		return Plan{}, PlanSettleResult{}, fmt.Errorf("session: settle: %w", err)
	}
	result.Revision = sm.plan.Revision + 1
	result.EventIDs = eventIDs
	ledger := PlanTransitionResult{
		Action:   SettleAction,
		StepID:   result.StepID,
		From:     result.From,
		To:       result.To,
		Revision: result.Revision,
	}
	if len(eventIDs) > 0 {
		ledger.EventID = eventIDs[0]
	}
	settled := result
	checked.Mutations = appendBoundedTail(checked.Mutations, PlanMutation{
		Mutation: settle.MutationID,
		Result:   ledger,
		Settle:   &settled,
	})
	// The audit trail rides inside the snapshot, so the settle owns the
	// serialized budget the same way authoring does.
	encoded, err := json.Marshal(checked)
	if err != nil {
		return Plan{}, PlanSettleResult{}, fmt.Errorf("session: encode plan for size validation: %w", err)
	}
	if len(encoded) > maxPlanV2SerializedBytes {
		return Plan{}, PlanSettleResult{}, fmt.Errorf(
			"session: plan is %d bytes; maximum is %d", len(encoded), maxPlanV2SerializedBytes,
		)
	}
	// A settle writes only operational fields; the guard keeps a future
	// field from silently revoking the user's approval.
	if diff := materialDiff(sm.plan, checked); len(diff) > 0 {
		return Plan{}, PlanSettleResult{}, fmt.Errorf(
			"session: settle would change material fields: %s", diff[0].Field,
		)
	}
	plan, _, err := sm.commitPlanLocked(checked, false)
	if err != nil {
		return Plan{}, PlanSettleResult{}, err
	}
	return plan, result, nil
}
