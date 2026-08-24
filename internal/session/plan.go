package session

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxPlanItems        = 64
	maxPlanContentRunes = 512
)

// PlanStatus is the lifecycle state of one model-managed plan item.
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "in_progress"
	PlanCompleted  PlanStatus = "completed"
	PlanCancelled  PlanStatus = "cancelled"
)

// PlanItem is one actionable step in the current session plan.
type PlanItem struct {
	Content string     `json:"content"`
	Status  PlanStatus `json:"status"`
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
func (sm *Manager) UpdatePlan(items []PlanItem) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	normalized, err := normalizePlanItems(items)
	if err != nil {
		return Plan{}, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

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
	if len(items) > maxPlanItems {
		return nil, fmt.Errorf("session: plan has %d items; maximum is %d", len(items), maxPlanItems)
	}
	out := make([]PlanItem, len(items))
	inProgress := 0
	for i, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		if item.Content == "" {
			return nil, fmt.Errorf("session: plan item %d content is empty", i+1)
		}
		if utf8.RuneCountInString(item.Content) > maxPlanContentRunes {
			return nil, fmt.Errorf("session: plan item %d exceeds %d characters", i+1, maxPlanContentRunes)
		}
		switch item.Status {
		case PlanPending, PlanCompleted, PlanCancelled:
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
	return out, nil
}
