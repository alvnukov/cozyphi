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

// Transition action names. Each names one lifecycle move over a step's status;
// the set is the whole transition vocabulary. Patch carries no transition
// authority; create sets only the initial status of a new contract and the
// legacy update path replaces a whole legacy snapshot. Within one v2 plan's
// life, the matrix below is the only audited writer of lifecycle history.
const (
	TransitionStart    = "start"
	TransitionComplete = "complete"
	TransitionBlock    = "block"
	TransitionResume   = "resume"
	TransitionCancel   = "cancel"
	TransitionReopen   = "reopen"
)

// transitionActionOrder drives deterministic error text and allowed-action
// listings; it must stay in sync with planTransitions.
var transitionActionOrder = []string{
	TransitionStart,
	TransitionComplete,
	TransitionBlock,
	TransitionResume,
	TransitionCancel,
	TransitionReopen,
}

// transitionSpec is one row of the lifecycle matrix: the target status and
// every status the action may leave.
type transitionSpec struct {
	to   PlanStatus
	from []PlanStatus
}

var planTransitions = map[string]transitionSpec{
	TransitionStart:    {to: PlanInProgress, from: []PlanStatus{PlanPending}},
	TransitionComplete: {to: PlanCompleted, from: []PlanStatus{PlanPending, PlanInProgress}},
	TransitionBlock:    {to: PlanBlocked, from: []PlanStatus{PlanPending, PlanInProgress}},
	TransitionResume:   {to: PlanInProgress, from: []PlanStatus{PlanBlocked}},
	TransitionCancel:   {to: PlanCancelled, from: []PlanStatus{PlanPending, PlanInProgress, PlanBlocked}},
	TransitionReopen:   {to: PlanPending, from: []PlanStatus{PlanCompleted, PlanCancelled}},
}

// PlanTransition is one lifecycle move addressed to a stable step id. The
// action's own payload carries its evidence contract; MutationID is the
// caller-supplied idempotency key whose replay returns the recorded result.
type PlanTransition struct {
	Action     string `json:"action"`
	StepID     string `json:"stepId"`
	MutationID string `json:"mutationId"`

	Outcome          string   `json:"outcome,omitempty"`          // complete
	Evidence         string   `json:"evidence,omitempty"`         // complete
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`     // complete
	NoEvidenceReason string   `json:"noEvidenceReason,omitempty"` // complete
	Blocker          string   `json:"blocker,omitempty"`          // block
	ResumeWhen       string   `json:"resumeWhen,omitempty"`       // block
	Reason           string   `json:"reason,omitempty"`           // cancel, reopen
}

// PlanTransitionResult is the compact answer to one transition: what moved,
// where it moved from and to, and the revision the move produced.
type PlanTransitionResult struct {
	Replayed bool       `json:"replayed,omitempty"`
	Action   string     `json:"action"`
	StepID   string     `json:"stepId"`
	From     PlanStatus `json:"from"`
	To       PlanStatus `json:"to"`
	Revision uint64     `json:"revision"`
	EventID  string     `json:"eventId,omitempty"`
}

// PlanEvent is one auditable lifecycle fact: the transition, its mutation id,
// and the payload the caller asserted. Clearing a completed step's fields on
// reopen never erases history — the completion stays on this record.
type PlanEvent struct {
	ID       string     `json:"id"`
	At       time.Time  `json:"at"`
	Mutation string     `json:"mutation"`
	Action   string     `json:"action"`
	StepID   string     `json:"stepId"`
	From     PlanStatus `json:"from"`
	To       PlanStatus `json:"to"`

	Outcome          string   `json:"outcome,omitempty"`
	Evidence         string   `json:"evidence,omitempty"`
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`
	NoEvidenceReason string   `json:"noEvidenceReason,omitempty"`
	Blocker          string   `json:"blocker,omitempty"`
	ResumeWhen       string   `json:"resumeWhen,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

// PlanMutation remembers the result one mutation id produced, so a retried
// call replays instead of applying twice. The ledger is durable plan state
// and shares the audit trail's bounded tail: a mutation retried after more
// than maxPlanEvents later transitions is judged by current state, not
// replayed.
type PlanMutation struct {
	Mutation string               `json:"mutation"`
	Result   PlanTransitionResult `json:"result"`
}

// TransitionPlan applies one validated lifecycle transition to the current v2
// plan. The move is guarded by the step's actual status, not by a revision:
// a transition addresses one stable id, so the state machine itself is the
// concurrency guard. A retried MutationID replays the recorded result without
// a new revision, a duplicate event, or duplicate evidence; the same id under
// different work is refused as a collision. Every applied move appends one
// audit event and lands durably through commitPlanLocked.
func (sm *Manager) TransitionPlan(
	transition PlanTransition,
	autoApprove bool,
) (Plan, PlanTransitionResult, error) {
	if sm == nil {
		return Plan{}, PlanTransitionResult{}, errors.New("session: plan manager is nil")
	}
	spec, ok := planTransitions[transition.Action]
	if !ok {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf(
			"session: unknown transition %q (use %s)",
			transition.Action, strings.Join(transitionActionOrder, ", "),
		)
	}
	normalizeTransition(&transition)
	if transition.StepID == "" {
		return Plan{}, PlanTransitionResult{}, errors.New("session: transition step id is required")
	}
	if err := validateMutationID(transition.MutationID); err != nil {
		return Plan{}, PlanTransitionResult{}, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.plan.Schema.IsV2() {
		return Plan{}, PlanTransitionResult{}, errors.New(
			"session: plan transitions require a v2 plan; send the full contract with action create",
		)
	}

	if recorded, found := findPlanMutation(sm.plan.Mutations, transition.MutationID); found {
		if recorded.Result.Action != transition.Action || recorded.Result.StepID != transition.StepID {
			return Plan{}, PlanTransitionResult{}, fmt.Errorf(
				"session: mutation id %q was already used for %s step %q",
				transition.MutationID, recorded.Result.Action, recorded.Result.StepID,
			)
		}
		replayed := recorded.Result
		replayed.Replayed = true
		return sm.plan.Clone(), replayed, nil
	}

	idx := findStepByID(sm.plan.Items, transition.StepID)
	if idx < 0 {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf("session: step %q not found", transition.StepID)
	}
	from := sm.plan.Items[idx].Status
	if !slices.Contains(spec.from, from) {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf(
			"session: step %q is %s; allowed actions: %s",
			transition.StepID, from, allowedTransitionsFrom(from),
		)
	}
	if err := validateTransitionPayload(transition); err != nil {
		return Plan{}, PlanTransitionResult{}, err
	}

	candidate := sm.plan.Clone()
	applyTransition(&candidate.Items[idx], spec, transition)
	checked, err := revalidatePatchedPlan(candidate)
	if err != nil {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf(
			"session: %s step %q: %w", transition.Action, transition.StepID, err,
		)
	}

	event := PlanEvent{
		ID:       sm.generateID(),
		At:       time.Now(),
		Mutation: transition.MutationID,
		Action:   transition.Action,
		StepID:   transition.StepID,
		From:     from,
		To:       spec.to,

		Outcome:          transition.Outcome,
		Evidence:         transition.Evidence,
		EvidenceRefs:     transition.EvidenceRefs,
		NoEvidenceReason: transition.NoEvidenceReason,
		Blocker:          transition.Blocker,
		ResumeWhen:       transition.ResumeWhen,
		Reason:           transition.Reason,
	}
	result := PlanTransitionResult{
		Action:   transition.Action,
		StepID:   transition.StepID,
		From:     from,
		To:       spec.to,
		Revision: sm.plan.Revision + 1,
		EventID:  event.ID,
	}
	checked.Events = appendBoundedTail(checked.Events, event)
	checked.Mutations = appendBoundedTail(
		checked.Mutations, PlanMutation{Mutation: transition.MutationID, Result: result},
	)
	// The audit trail rides inside the snapshot, so the transition itself
	// owns the serialized budget the same way authoring does.
	encoded, err := json.Marshal(checked)
	if err != nil {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf("session: encode plan for size validation: %w", err)
	}
	if len(encoded) > maxPlanV2SerializedBytes {
		return Plan{}, PlanTransitionResult{}, fmt.Errorf(
			"session: plan is %d bytes; maximum is %d", len(encoded), maxPlanV2SerializedBytes,
		)
	}

	plan, err := sm.commitPlanLocked(checked, autoApprove)
	if err != nil {
		return Plan{}, PlanTransitionResult{}, err
	}
	return plan, result, nil
}

// normalizeTransition trims every payload field in place; validation and the
// applied result then see the same text the audit event records.
func normalizeTransition(tr *PlanTransition) {
	tr.StepID = strings.TrimSpace(tr.StepID)
	tr.MutationID = strings.TrimSpace(tr.MutationID)
	tr.Outcome = strings.TrimSpace(tr.Outcome)
	tr.Evidence = strings.TrimSpace(tr.Evidence)
	tr.NoEvidenceReason = strings.TrimSpace(tr.NoEvidenceReason)
	tr.Blocker = strings.TrimSpace(tr.Blocker)
	tr.ResumeWhen = strings.TrimSpace(tr.ResumeWhen)
	tr.Reason = strings.TrimSpace(tr.Reason)
	for i, ref := range tr.EvidenceRefs {
		tr.EvidenceRefs[i] = strings.TrimSpace(ref)
	}
}

func validateMutationID(id string) error {
	if id == "" {
		return errors.New("session: transition mutation id is required")
	}
	if utf8.RuneCountInString(id) > maxPlanStepIDRunes {
		return fmt.Errorf("session: transition mutation id exceeds %d characters", maxPlanStepIDRunes)
	}
	if !planStepIDPattern.MatchString(id) {
		return fmt.Errorf(
			"session: transition mutation id %q must be a lowercase slug of letters, digits, '.', '_' or '-'",
			id,
		)
	}
	return nil
}

// validateTransitionPayload enforces each action's own contract: complete
// carries an outcome plus evidence (or an explicit no-evidence reason), block
// names its blocker and resume condition, and cancel/reopen explain
// themselves. Fields another action owns are refused rather than dropped.
func validateTransitionPayload(tr PlanTransition) error {
	if foreign := transitionForeignFields(tr); len(foreign) > 0 {
		return fmt.Errorf("session: %s step %q takes no %s", tr.Action, tr.StepID, foreign[0])
	}
	switch tr.Action {
	case TransitionComplete:
		if tr.Outcome == "" {
			return fmt.Errorf("session: complete step %q: outcome is required", tr.StepID)
		}
		hasEvidence := tr.Evidence != "" || len(tr.EvidenceRefs) > 0
		switch {
		case !hasEvidence && tr.NoEvidenceReason == "":
			return fmt.Errorf(
				"session: complete step %q: requires evidence, evidence_refs, or no_evidence_reason",
				tr.StepID,
			)
		case hasEvidence && tr.NoEvidenceReason != "":
			return fmt.Errorf(
				"session: complete step %q: no_evidence_reason is only allowed without evidence",
				tr.StepID,
			)
		}
		for i, ref := range tr.EvidenceRefs {
			if ref == "" {
				return fmt.Errorf("session: complete step %q: evidence ref %d is empty", tr.StepID, i+1)
			}
		}
	case TransitionBlock:
		if tr.Blocker == "" {
			return fmt.Errorf("session: block step %q: blocker is required", tr.StepID)
		}
		if tr.ResumeWhen == "" {
			return fmt.Errorf("session: block step %q: resume_when is required", tr.StepID)
		}
	case TransitionCancel, TransitionReopen:
		if tr.Reason == "" {
			return fmt.Errorf("session: %s step %q: reason is required", tr.Action, tr.StepID)
		}
	}
	if utf8.RuneCountInString(tr.NoEvidenceReason) > maxPlanReasonRunes {
		return fmt.Errorf(
			"session: complete step %q: no_evidence_reason exceeds %d characters",
			tr.StepID, maxPlanReasonRunes,
		)
	}
	if utf8.RuneCountInString(tr.Reason) > maxPlanReasonRunes {
		return fmt.Errorf(
			"session: %s step %q: reason exceeds %d characters",
			tr.Action, tr.StepID, maxPlanReasonRunes,
		)
	}
	return nil
}

// transitionForeignFields collects every populated payload field outside the
// action's own contract. The result is sorted so the reported offender is
// deterministic. The table is pinned to the struct's field list by
// TestTransitionForeignTableCoversEveryField.
func transitionForeignFields(tr PlanTransition) []string {
	allowed := map[string][]string{
		TransitionStart:  nil,
		TransitionResume: nil,
		TransitionCancel: {"reason"},
		TransitionReopen: {"reason"},
		TransitionBlock:  {"blocker", "resumeWhen"},
		TransitionComplete: {
			"outcome", "evidence", "evidenceRefs", "noEvidenceReason",
		},
	}
	populated := []struct {
		name string
		set  bool
	}{
		{"outcome", tr.Outcome != ""},
		{"evidence", tr.Evidence != ""},
		{"evidenceRefs", tr.EvidenceRefs != nil},
		{"noEvidenceReason", tr.NoEvidenceReason != ""},
		{"blocker", tr.Blocker != ""},
		{"resumeWhen", tr.ResumeWhen != ""},
		{"reason", tr.Reason != ""},
	}
	var foreign []string
	for _, field := range populated {
		if field.set && !slices.Contains(allowed[tr.Action], field.name) {
			foreign = append(foreign, field.name)
		}
	}
	slices.Sort(foreign)
	return foreign
}

// applyTransition writes the move onto the step. Fields the move owns are
// replaced wholesale; leaving a status clears the fields that described it —
// resume forgets the blocker, reopen forgets the completion record — because
// the audit event already holds them.
func applyTransition(item *PlanItem, spec transitionSpec, tr PlanTransition) {
	item.Status = spec.to
	switch tr.Action {
	case TransitionComplete:
		item.Outcome = tr.Outcome
		item.Evidence = tr.Evidence
		item.EvidenceRefs = tr.EvidenceRefs
	case TransitionBlock:
		item.Blocker = tr.Blocker
		item.ResumeWhen = tr.ResumeWhen
	case TransitionResume, TransitionCancel:
		item.Blocker = ""
		item.ResumeWhen = ""
	case TransitionReopen:
		item.Outcome = ""
		item.Evidence = ""
		item.EvidenceRefs = nil
		item.Blocker = ""
		item.ResumeWhen = ""
	}
}

// allowedTransitionsFrom lists the actions a status permits, derived from the
// one matrix so the error can never drift from the enforcement.
func allowedTransitionsFrom(status PlanStatus) string {
	var allowed []string
	for _, action := range transitionActionOrder {
		if slices.Contains(planTransitions[action].from, status) {
			allowed = append(allowed, action)
		}
	}
	return strings.Join(allowed, ", ")
}

func findPlanMutation(mutations []PlanMutation, id string) (PlanMutation, bool) {
	for _, recorded := range mutations {
		if recorded.Mutation == id {
			return recorded, true
		}
	}
	return PlanMutation{}, false
}

// appendBoundedTail appends one record and keeps the most recent
// maxPlanEvents entries, so a long-lived plan snapshot stays bounded.
func appendBoundedTail[T any](list []T, entry T) []T {
	list = append(list, entry)
	if excess := len(list) - maxPlanEvents; excess > 0 {
		list = list[excess:]
	}
	return list
}
