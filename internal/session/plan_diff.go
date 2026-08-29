package session

import (
	"fmt"
	"slices"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// MaterialChange names how one material field moved; the set below is the
// whole diff vocabulary.
type MaterialChange string

const (
	MaterialChanged   MaterialChange = "changed"
	MaterialAdded     MaterialChange = "added"
	MaterialRemoved   MaterialChange = "removed"
	MaterialReordered MaterialChange = "reordered"
)

// PlanMaterialChange names one material difference between two plan
// snapshots — the kind of change that revokes the user's approval. Target is
// "plan" or a stable step id (legacy steps carry no ids and are labeled by
// their 1-based ordinal). Detail carries only what the field's identity
// needs: directive text, or the old and new step type. It never echoes whole
// plan prose.
type PlanMaterialChange struct {
	Target string         `json:"target"`
	Field  string         `json:"field"`
	Change MaterialChange `json:"change"`
	Detail string         `json:"detail,omitempty"`
}

// materialDiff returns every material difference between two snapshots in a
// deterministic order: plan prose, directives, then steps keyed by stable
// identity. The same table decides approval — commitPlanLocked drops the
// user's approval exactly when this list is non-empty — so the decision and
// its explanation cannot drift apart.
//
// Material (approval-revoking) fields: plan goal, approach, success
// criteria, and constraints; step action (content), type, done_when, risk,
// and the just-in-time approval posture; added, removed, or reordered steps.
// Everything else is operational and keeps approval: status, outcome,
// evidence and refs, blocker, resume_when, note, wording-only why, working
// context, result metadata, and the audit ledger. Working context and why
// are prose the model rewrites as work proceeds; risk and the JIT posture
// change what the user is asked to accept, so they stay material.
func materialDiff(old, next Plan) []PlanMaterialChange {
	var diff []PlanMaterialChange
	if old.Goal != next.Goal {
		diff = append(diff, PlanMaterialChange{Target: "plan", Field: "goal", Change: MaterialChanged})
	}
	if old.Approach != next.Approach {
		diff = append(diff, PlanMaterialChange{Target: "plan", Field: "approach", Change: MaterialChanged})
	}
	diff = append(diff, directiveDiff("successCriteria", old.SuccessCriteria, next.SuccessCriteria)...)
	diff = append(diff, directiveDiff("constraints", old.Constraints, next.Constraints)...)
	diff = append(diff, stepsDiff(old.Items, next.Items)...)
	return diff
}

// directiveDiff diffs two directive lists by exact text, their identity.
// Rewording one entry reports as a removal plus an addition; removals list
// in old order first, then additions in new order.
func directiveDiff(field string, old, next []string) []PlanMaterialChange {
	var diff []PlanMaterialChange
	for _, entry := range old {
		if !slices.Contains(next, entry) {
			diff = append(diff, PlanMaterialChange{
				Target: "plan", Field: field, Change: MaterialRemoved, Detail: redact.Redact(entry),
			})
		}
	}
	for _, entry := range next {
		if !slices.Contains(old, entry) {
			diff = append(diff, PlanMaterialChange{
				Target: "plan", Field: field, Change: MaterialAdded, Detail: redact.Redact(entry),
			})
		}
	}
	return diff
}

// stepsDiff pairs steps by stable id and reports removals, additions, and
// per-step field changes, then one reorder entry when the surviving steps
// kept their set but moved. Legacy steps carry no ids: they pair by 1-based
// ordinal, so an appended or truncated tail reports as added or removed,
// while a middle insertion shifts the ordinals after it — v2 plans, whose
// ids are required, are always paired exactly.
func stepsDiff(old, next []PlanItem) []PlanMaterialChange {
	oldKeys := make([]string, len(old))
	oldByKey := make(map[string]PlanItem, len(old))
	for i, item := range old {
		oldKeys[i] = stepDiffKey(item, i)
		oldByKey[oldKeys[i]] = item
	}
	nextKeys := make([]string, len(next))
	nextByKey := make(map[string]PlanItem, len(next))
	for i, item := range next {
		nextKeys[i] = stepDiffKey(item, i)
		nextByKey[nextKeys[i]] = item
	}

	var diff []PlanMaterialChange
	for _, key := range oldKeys {
		if _, ok := nextByKey[key]; !ok {
			diff = append(diff, PlanMaterialChange{Target: key, Field: "step", Change: MaterialRemoved})
		}
	}
	for _, key := range nextKeys {
		before, ok := oldByKey[key]
		if !ok {
			diff = append(diff, PlanMaterialChange{Target: key, Field: "step", Change: MaterialAdded})
			continue
		}
		diff = append(diff, stepFieldDiff(key, before, nextByKey[key])...)
	}
	if sameKeySet(oldKeys, nextKeys) && !slices.Equal(oldKeys, nextKeys) {
		diff = append(diff, PlanMaterialChange{Target: "plan", Field: "steps", Change: MaterialReordered})
	}
	return diff
}

// stepFieldDiff reports one step's material field changes in a fixed order:
// the action, its type, its exit condition, its risk, and its approval
// posture.
func stepFieldDiff(key string, old, next PlanItem) []PlanMaterialChange {
	var diff []PlanMaterialChange
	if old.Content != next.Content {
		diff = append(diff, PlanMaterialChange{Target: key, Field: "content", Change: MaterialChanged})
	}
	if old.Type != next.Type {
		diff = append(diff, PlanMaterialChange{
			Target: key,
			Field:  "type",
			Change: MaterialChanged,
			Detail: fmt.Sprintf("%s to %s", old.Type, next.Type),
		})
	}
	if old.DoneWhen != next.DoneWhen {
		diff = append(diff, PlanMaterialChange{Target: key, Field: "doneWhen", Change: MaterialChanged})
	}
	if old.Risk != next.Risk {
		diff = append(diff, PlanMaterialChange{Target: key, Field: "risk", Change: MaterialChanged})
	}
	if old.JIT != next.JIT {
		diff = append(diff, PlanMaterialChange{Target: key, Field: "jit", Change: MaterialChanged})
	}
	return diff
}

func stepDiffKey(item PlanItem, position int) string {
	if item.ID != "" {
		return item.ID
	}
	return fmt.Sprintf("step %d", position+1)
}

func sameKeySet(old, next []string) bool {
	if len(old) != len(next) {
		return false
	}
	set := make(map[string]struct{}, len(old))
	for _, key := range old {
		set[key] = struct{}{}
	}
	for _, key := range next {
		if _, ok := set[key]; !ok {
			return false
		}
	}
	return true
}
