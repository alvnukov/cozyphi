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

const (
	maxPlanItems           = 32
	maxPlanContentRunes    = 256
	maxPlanNoteRunes       = 256
	maxPlanEvidenceRunes   = 256
	maxPlanSerializedBytes = 16 * 1024

	// Previous releases allowed these values. Loading remains compatible;
	// only newly authored snapshots use the tighter model-facing budget.
	legacyMaxPlanItems        = 64
	legacyMaxPlanContentRunes = 512
)

// ReplacePlan validates and durably appends a complete plan snapshot, atomically
// replacing whatever the manager currently holds under one lock. It never compares
// against a model-supplied revision: the harness owner of the plan is the only
// writer, so a whole-snapshot replace cannot produce a lost update. Plan metadata
// never becomes the session leaf, so it cannot perturb branching, compaction, or
// the provider message context.
func (sm *Manager) ReplacePlan(items []PlanItem) (Plan, error) {
	return sm.ReplacePlanWithAutoApprove(items, false)
}

// ReplacePlanWithAutoApprove atomically replaces a plan and applies the current
// auto-approval policy to the same durable snapshot. Active plans are approved
// when autoApprove is true; plans with no remaining work are always unapproved.
func (sm *Manager) ReplacePlanWithAutoApprove(items []PlanItem, autoApprove bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	return sm.replacePlanLocked(items, autoApprove)
}

// RenamePlanStepTypes durably migrates type references without changing plan
// approval, content, status, note, or evidence. It is the session half of a
// global settings rename transaction.
func (sm *Manager) RenamePlanStepTypes(renames map[StepType]StepType) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	for from, to := range renames {
		if from == "" || to == "" {
			return Plan{}, errors.New("session: step type rename cannot be empty")
		}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	items := slices.Clone(sm.plan.Items)
	changed := false
	for i := range items {
		if renamed, ok := renames[items[i].Type]; ok && renamed != items[i].Type {
			items[i].Type = renamed
			changed = true
		}
	}
	if !changed {
		return sm.plan.Clone(), nil
	}
	plan := Plan{
		Revision:  sm.plan.Revision + 1,
		UpdatedAt: time.Now(),
		Items:     items,
		Approved:  sm.plan.Approved,
	}
	return sm.persistPlanLocked(plan)
}

// replacePlanLocked normalizes, validates, and persists the snapshot. It keeps
// approval when only active step status/metadata change, drops it when the step
// content or type set changes, and always closes approval when no work remains.
func (sm *Manager) replacePlanLocked(items []PlanItem, autoApprove bool) (Plan, error) {
	normalized, err := normalizePlanItems(items)
	if err != nil {
		return Plan{}, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	approved := sm.plan.Approved
	if !sameStepBodies(sm.plan.Items, normalized) {
		approved = false
	}
	if !planItemsHaveActiveWork(normalized) {
		approved = false
	} else if autoApprove {
		approved = true
	}

	plan := Plan{
		Revision:  sm.plan.Revision + 1,
		UpdatedAt: time.Now(),
		Items:     normalized,
		Approved:  approved,
	}
	return sm.persistPlanLocked(plan)
}

// PlanStatus is the lifecycle state of one model-managed plan item.
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "in_progress"
	PlanBlocked    PlanStatus = "blocked"
	PlanCompleted  PlanStatus = "completed"
	PlanCancelled  PlanStatus = "cancelled"
)

// StepType classifies what a plan item is allowed to do. The plan gate maps
// a step's type onto the set of tools it may call.
type StepType string

const (
	StepExplore   StepType = "explore"   // read, grep, find, ls
	StepEdit      StepType = "edit"      // + write, edit
	StepRun       StepType = "run"       // + bash
	StepDelegate  StepType = "delegate"  // + agent_*
	StepIntegrate StepType = "integrate" // + mcp_*
)

// PlanItem is one actionable step in the current session plan.
type PlanItem struct {
	Content  string     `json:"content"`
	Status   PlanStatus `json:"status"`
	Type     StepType   `json:"type,omitempty"`
	Note     string     `json:"note,omitempty"`
	Evidence string     `json:"evidence,omitempty"`
}

// Plan is the latest durable, ordered plan snapshot for a session.
type Plan struct {
	Revision  uint64     `json:"revision"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Items     []PlanItem `json:"items"`
	Approved  bool       `json:"approved,omitempty"`
}

// Clone returns a snapshot whose items do not alias manager state.
func (p Plan) Clone() Plan {
	p.Items = slices.Clone(p.Items)
	return p
}

// HasActiveWork reports whether the plan contains a step that is not completed
// or cancelled. An empty plan has no active work.
func (p Plan) HasActiveWork() bool { return planItemsHaveActiveWork(p.Items) }

func planItemsHaveActiveWork(items []PlanItem) bool {
	for _, item := range items {
		if item.Status != PlanCompleted && item.Status != PlanCancelled {
			return true
		}
	}
	return false
}

// PlanEntry stores a plan snapshot in the append-only session log without
// moving the conversational leaf or entering the model context.
type PlanEntry struct {
	SessionBaseEntry
	Plan Plan `json:"plan"`
}

func (PlanEntry) GetType() string    { return EntryPlan }
func (p PlanEntry) GetID() string    { return p.ID }
func (PlanEntry) GetParent() *string { return nil }

// Plan returns the latest session plan snapshot.
func (sm *Manager) Plan() Plan {
	if sm == nil {
		return Plan{}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.plan.Clone()
}

// sameStepBodies reports whether two snapshots differ only in step metadata
// (status/note/evidence) while keeping the same ordered content+type set.
func sameStepBodies(old, new []PlanItem) bool {
	if len(old) != len(new) {
		return false
	}
	for i := range old {
		if old[i].Content != new[i].Content || old[i].Type != new[i].Type {
			return false
		}
	}
	return true
}

// SetPlanApproved flips the user-owned approval flag and durably appends the
// new snapshot. Items stay untouched; only the revision moves.
func (sm *Manager) SetPlanApproved(approved bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	plan := Plan{
		Revision:  sm.plan.Revision + 1,
		UpdatedAt: time.Now(),
		Items:     sm.plan.Items,
		Approved:  approved,
	}
	return sm.persistPlanLocked(plan)
}

// ClearPlan drops the durable plan and resets its revision counter to zero.
func (sm *Manager) ClearPlan() (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	plan := Plan{UpdatedAt: time.Now()}
	return sm.persistPlanLocked(plan)
}

// persistPlanLocked appends the plan snapshot and rolls back on flush failure.
// The caller holds sm.mu.
func (sm *Manager) persistPlanLocked(plan Plan) (Plan, error) {
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: sm.generateID(), Timestamp: plan.UpdatedAt},
		Plan:             plan,
	}
	previousLen := len(sm.entries)
	sm.entries = append(sm.entries, entry)
	sm.byIDs[entry.ID] = entry
	if sm.config.shouldFlush {
		if err := sm.flush(entry); err != nil {
			sm.entries = sm.entries[:previousLen]
			delete(sm.byIDs, entry.ID)
			return Plan{}, fmt.Errorf("session: persist plan: %w", err)
		}
	}
	sm.plan = plan
	return plan.Clone(), nil
}

func normalizePlanItems(items []PlanItem) ([]PlanItem, error) {
	return normalizePlanItemsWithLimits(items, maxPlanItems, maxPlanContentRunes, true)
}

func normalizeLoadedPlanItems(items []PlanItem) ([]PlanItem, error) {
	return normalizePlanItemsWithLimits(items, legacyMaxPlanItems, legacyMaxPlanContentRunes, false)
}

func normalizePlanItemsWithLimits(
	items []PlanItem,
	maxItems int,
	maxContentRunes int,
	enforceSerializedLimit bool,
) ([]PlanItem, error) {
	if len(items) > maxItems {
		return nil, fmt.Errorf("session: plan has %d items; maximum is %d", len(items), maxItems)
	}
	out := make([]PlanItem, len(items))
	inProgress := 0
	for i, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		item.Note = strings.TrimSpace(item.Note)
		item.Evidence = strings.TrimSpace(item.Evidence)
		if item.Content == "" {
			return nil, fmt.Errorf("session: plan item %d content is empty", i+1)
		}
		if utf8.RuneCountInString(item.Content) > maxContentRunes {
			return nil, fmt.Errorf("session: plan item %d content exceeds %d characters", i+1, maxContentRunes)
		}
		if utf8.RuneCountInString(item.Note) > maxPlanNoteRunes {
			return nil, fmt.Errorf("session: plan item %d note exceeds %d characters", i+1, maxPlanNoteRunes)
		}
		if utf8.RuneCountInString(item.Evidence) > maxPlanEvidenceRunes {
			return nil, fmt.Errorf("session: plan item %d evidence exceeds %d characters", i+1, maxPlanEvidenceRunes)
		}
		switch item.Status {
		case PlanPending, PlanBlocked, PlanCompleted, PlanCancelled:
		case PlanInProgress:
			inProgress++
		default:
			return nil, fmt.Errorf("session: plan item %d has invalid status %q", i+1, item.Status)
		}
		out[i] = item
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("session: plan has %d in_progress items; maximum is 1", inProgress)
	}
	if enforceSerializedLimit {
		encoded, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("session: encode plan for size validation: %w", err)
		}
		if len(encoded) > maxPlanSerializedBytes {
			return nil, fmt.Errorf("session: plan is %d bytes; maximum is %d", len(encoded), maxPlanSerializedBytes)
		}
	}
	return out, nil
}
