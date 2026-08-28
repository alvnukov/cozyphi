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

// Attempt statuses: the bounded terminal outcomes of one accepted gateable
// tool call. A failed, canceled or lost attempt is evidence of work, never a
// step transition.
const (
	AttemptSuccess  = "success"
	AttemptFailed   = "failed"
	AttemptCanceled = "canceled"
	AttemptLost     = "lost"
)

// attemptRefPrefix marks an evidence ref that cites a recorded attempt by its
// tool call id ("call:<callId>"). Refs without the prefix are model-authored
// artifacts and are never validated against attempts; the model-facing prose
// in plantool and plangate spells the same form out in words.
const attemptRefPrefix = "call:"

const (
	// The per-step attempt history is a bounded tail — the most recent calls
	// are the citable evidence — and every field in it is capped so the
	// whole snapshot stays under one serialized budget.
	maxPlanAttemptsPerStep     = 4
	maxPlanAttemptCallIDRunes  = 128
	maxPlanAttemptToolRunes    = 64
	maxPlanAttemptSummaryRunes = 128
)

// PlanAttempt is one bounded record of a tool call the harness accepted
// against a step: the call's identity into the transcript, its terminal
// status, and a harness-truncated summary. Full tool output never lands in
// the plan — the transcript holds it by call id.
type PlanAttempt struct {
	CallID  string    `json:"callId"`
	Tool    string    `json:"tool"`
	Status  string    `json:"status"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
}

// RecordPlanAttempt durably upserts one attempt onto the addressed step. The
// call id is the record's identity, so reconciliation that re-reports the
// same call updates it in place instead of duplicating it. Attempts are
// operational evidence: they never move lifecycle state or approval.
func (sm *Manager) RecordPlanAttempt(stepID string, attempt PlanAttempt) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return Plan{}, errors.New("session: attempt step id is required")
	}
	if err := normalizePlanAttempt(&attempt); err != nil {
		return Plan{}, fmt.Errorf("session: record attempt on step %q: %w", stepID, err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.plan.Schema.IsV2() {
		return Plan{}, errors.New(
			"session: plan attempts require a v2 plan; send the full contract with action create",
		)
	}
	idx := findStepByID(sm.plan.Items, stepID)
	if idx < 0 {
		return Plan{}, fmt.Errorf("session: step %q not found", stepID)
	}

	plan := sm.plan.Clone()
	plan.Items[idx].Attempts = upsertPlanAttempt(plan.Items[idx].Attempts, attempt)
	plan.Revision = sm.plan.Revision + 1
	plan.UpdatedAt = attempt.At
	// The attempt history rides inside the snapshot, so the record itself
	// owns the serialized budget the same way authoring does.
	encoded, err := json.Marshal(plan)
	if err != nil {
		return Plan{}, fmt.Errorf("session: encode plan for size validation: %w", err)
	}
	if len(encoded) > maxPlanV2SerializedBytes {
		return Plan{}, fmt.Errorf(
			"session: plan is %d bytes; maximum is %d",
			len(encoded),
			maxPlanV2SerializedBytes,
		)
	}
	return sm.persistPlanLocked(plan)
}

// normalizePlanAttempt validates and bounds one incoming record. The summary
// is harness-authored, so an over-long one is truncated, not rejected — the
// reject-don't-truncate policy in boundStepV2Fields is for foreign files this
// harness never wrote.
func normalizePlanAttempt(attempt *PlanAttempt) error {
	attempt.CallID = strings.TrimSpace(attempt.CallID)
	attempt.Tool = strings.TrimSpace(attempt.Tool)
	if attempt.CallID == "" {
		return errors.New("call id is required")
	}
	if utf8.RuneCountInString(attempt.CallID) > maxPlanAttemptCallIDRunes {
		return fmt.Errorf("call id exceeds %d characters", maxPlanAttemptCallIDRunes)
	}
	if attempt.Tool == "" {
		return errors.New("tool is required")
	}
	if utf8.RuneCountInString(attempt.Tool) > maxPlanAttemptToolRunes {
		return fmt.Errorf("tool exceeds %d characters", maxPlanAttemptToolRunes)
	}
	if !validPlanAttemptStatus(attempt.Status) {
		return fmt.Errorf("unknown attempt status %q", attempt.Status)
	}
	attempt.Summary = boundAttemptSummary(strings.TrimSpace(attempt.Summary))
	if attempt.At.IsZero() {
		attempt.At = time.Now()
	}
	return nil
}

func validPlanAttemptStatus(status string) bool {
	switch status {
	case AttemptSuccess, AttemptFailed, AttemptCanceled, AttemptLost:
		return true
	}
	return false
}

// boundAttemptSummary truncates the summary to the evidence budget, marking
// the cut so a bounded record is always distinguishable from a short one.
func boundAttemptSummary(summary string) string {
	if utf8.RuneCountInString(summary) <= maxPlanAttemptSummaryRunes {
		return summary
	}
	runes := []rune(summary)
	return string(runes[:maxPlanAttemptSummaryRunes-3]) + "..."
}

// upsertPlanAttempt folds one record into the step's bounded tail: the same
// call id updates in place, a new call id appends, and the oldest records
// drop off past the bound. Evidence stays honest two ways: reconciliation of
// one call is one record, never a duplicate (the call id is the identity),
// and eviction from the tail loses nothing durable — the session transcript
// is the append-only full audit that holds every call by id; the tail is only
// the compact index the model cites from.
func upsertPlanAttempt(attempts []PlanAttempt, attempt PlanAttempt) []PlanAttempt {
	for i := range attempts {
		if attempts[i].CallID == attempt.CallID {
			attempts[i] = attempt
			return attempts
		}
	}
	attempts = append(attempts, attempt)
	if excess := len(attempts) - maxPlanAttemptsPerStep; excess > 0 {
		attempts = attempts[excess:]
	}
	return attempts
}

// validateCompleteAttemptRefs checks the complete action's attempt citations:
// every call: ref must name a successful attempt persisted on the completing
// step, so an outcome cannot lean on another step's work, a failed call, or a
// call that never happened. Refs without the prefix are model-authored
// artifacts and pass untouched.
func validateCompleteAttemptRefs(tr PlanTransition, item PlanItem) error {
	if tr.Action != TransitionComplete {
		return nil
	}
	for _, ref := range tr.EvidenceRefs {
		if !strings.HasPrefix(ref, attemptRefPrefix) {
			continue
		}
		callID := strings.TrimSpace(strings.TrimPrefix(ref, attemptRefPrefix))
		if !slices.ContainsFunc(item.Attempts, func(a PlanAttempt) bool {
			return a.CallID == callID && a.Status == AttemptSuccess
		}) {
			return fmt.Errorf(
				"session: complete step %q: evidence ref %q is not a successful attempt of this step",
				tr.StepID, ref,
			)
		}
	}
	return nil
}
