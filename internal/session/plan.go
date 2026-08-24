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

// ErrPlanRevisionConflict means an update was based on a stale snapshot.
var ErrPlanRevisionConflict = errors.New("session: plan revision conflict")

// PlanStatus is the lifecycle state of one model-managed plan item.
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "in_progress"
	PlanBlocked    PlanStatus = "blocked"
	PlanCompleted  PlanStatus = "completed"
	PlanCancelled  PlanStatus = "cancelled"
)

// PlanItem is one actionable step in the current session plan.
type PlanItem struct {
	Content  string     `json:"content"`
	Status   PlanStatus `json:"status"`
	Note     string     `json:"note,omitempty"`
	Evidence string     `json:"evidence,omitempty"`
}

// Plan is the latest durable, ordered plan snapshot for a session.
type Plan struct {
	Revision  uint64     `json:"revision"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Items     []PlanItem `json:"items"`
}

// Clone returns a snapshot whose items do not alias manager state.
func (p Plan) Clone() Plan {
	p.Items = slices.Clone(p.Items)
	return p
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

// UpdatePlan validates and durably appends a complete plan snapshot. Plan
// metadata never becomes the session leaf, so it cannot perturb branching,
// compaction, or the provider message context.
func (sm *Manager) UpdatePlan(expectedRevision uint64, items []PlanItem) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	normalized, err := normalizePlanItems(items)
	if err != nil {
		return Plan{}, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.plan.Revision != expectedRevision {
		return Plan{}, fmt.Errorf(
			"%w: expected %d, current %d; call plan with action=get and retry",
			ErrPlanRevisionConflict,
			expectedRevision,
			sm.plan.Revision,
		)
	}

	plan := Plan{Revision: sm.plan.Revision + 1, UpdatedAt: time.Now(), Items: normalized}
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
