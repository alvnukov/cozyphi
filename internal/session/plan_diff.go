package session

import (
	"fmt"
	"slices"
	"strings"

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

// MaterialDiff returns every material difference between two snapshots in a
// deterministic order: plan prose, directives, then steps keyed by stable
// identity. It is the same table that decides approval — commitPlanLocked
// drops the user's approval exactly when this list is non-empty — so the
// decision and its explanation cannot drift apart. Rendering surfaces read
// it for the reapproval diff without mutating either snapshot.
//
// Material (approval-revoking) fields: plan goal, approach, success
// criteria, and constraints; step action (content), type, done_when, risk,
// and the just-in-time approval posture; added, removed, or reordered steps.
// Everything else is operational and keeps approval: status, outcome,
// evidence and refs, blocker, resume_when, note, wording-only why, working
// context, result metadata, and the audit ledger. Working context and why
// are prose the model rewrites as work proceeds; risk and the JIT posture
// change what the user is asked to accept, so they stay material.
func MaterialDiff(old, next Plan) []PlanMaterialChange {
	var diff []PlanMaterialChange
	if old.Goal != next.Goal {
		diff = append(diff, PlanMaterialChange{Target: "plan", Field: "goal", Change: MaterialChanged})
	}
	if old.Approach != next.Approach {
		diff = append(diff, PlanMaterialChange{Target: "plan", Field: "approach", Change: MaterialChanged})
	}
	diff = append(diff, directiveDiff("successCriteria", old.SuccessCriteria, next.SuccessCriteria)...)
	diff = append(diff, directiveDiff("constraints", old.Constraints, next.Constraints)...)
	diff = append(diff, modelsByTypeDiff(old.ModelsByType, next.ModelsByType)...)
	diff = append(diff, actionsDiff("plan", old.Actions, next.Actions)...)
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

// modelsByTypeDiff diffs the per-step-type model map by key. Keys are a
// closed set, so every change is a changed value (an added or removed key
// can only come from a file this harness never writes).
func modelsByTypeDiff(old, next map[StepType]string) []PlanMaterialChange {
	keys := make([]string, 0, len(old)+len(next))
	for stepType := range old {
		keys = append(keys, string(stepType))
	}
	for stepType := range next {
		if _, ok := old[stepType]; !ok {
			keys = append(keys, string(stepType))
		}
	}
	slices.Sort(keys)
	var diff []PlanMaterialChange
	for _, key := range keys {
		stepType := StepType(key)
		oldModel, inOld := old[stepType]
		newModel, inNext := next[stepType]
		if inOld == inNext && oldModel == newModel {
			continue
		}
		detail := fmt.Sprintf("%s: %s to %s", stepType, oldModel, newModel)
		diff = append(
			diff,
			PlanMaterialChange{Target: "plan", Field: "modelsByType", Change: MaterialChanged, Detail: detail},
		)
	}
	return diff
}

// actionsDiff pairs actions by position — an action list is a short ordered
// set of per-event hooks, not an identity-addressed collection — and names
// the index where the definitions diverge. The disabled-skill set counts:
// a toggle changes what runs, so it revokes approval; the off names ride in
// the detail so the change reads as what it is. Run history is operational
// and never diffed.
func actionsDiff(target string, old, next []PlanAction) []PlanMaterialChange {
	var diff []PlanMaterialChange
	for i := range old {
		if i >= len(next) {
			diff = append(diff, PlanMaterialChange{
				Target: target, Field: "actions", Change: MaterialRemoved,
				Detail: planActionDiffDetail(i+1, old[i]),
			})
			continue
		}
		if !sameActionDefinition(old[i], next[i]) {
			diff = append(diff, PlanMaterialChange{
				Target: target, Field: "actions", Change: MaterialChanged,
				Detail: actionChangeDetail(i+1, old[i], next[i]),
			})
		}
	}
	for i := len(old); i < len(next); i++ {
		diff = append(diff, PlanMaterialChange{
			Target: target, Field: "actions", Change: MaterialAdded,
			Detail: planActionDiffDetail(i+1, next[i]),
		})
	}
	return diff
}

// sameActionDefinition reports whether two actions match in every field the
// approval covers, the disabled set included. PlanActionEqual stays blind to
// the disabled set on purpose, so a toggle cannot retire run history.
func sameActionDefinition(a, b PlanAction) bool {
	return PlanActionEqual(a, b) && slices.Equal(a.DisabledSkills, b.DisabledSkills)
}

// planActionDiffDetail names one action for the approval diff, with its off
// marks when present, so a skill toggle is legible in the change list.
func planActionDiffDetail(index int, action PlanAction) string {
	detail := planActionName(index, action)
	if len(action.DisabledSkills) > 0 {
		detail += fmt.Sprintf(" (off: %s)", strings.Join(action.DisabledSkills, ", "))
	}
	return detail
}

// actionChangeDetail names the position's old definition and, when the off
// marks moved, which way: the approval must read a toggle as a toggle, not
// as a bare "changed".
func actionChangeDetail(index int, old, next PlanAction) string {
	if slices.Equal(old.DisabledSkills, next.DisabledSkills) {
		return planActionDiffDetail(index, old)
	}
	return fmt.Sprintf(
		"%s (off: %s to %s)", planActionName(index, old), offList(old.DisabledSkills), offList(next.DisabledSkills),
	)
}

func planActionName(index int, action PlanAction) string {
	return fmt.Sprintf("%d: %s %s", index, action.Event, action.Type)
}

func offList(disabled []string) string {
	if len(disabled) == 0 {
		return "none"
	}
	return strings.Join(disabled, ", ")
}

// stepsDiff pairs steps by stable id and reports removals, additions, and
// per-step field changes, then one reorder entry when the surviving steps
// kept their set but moved. Legacy steps carry no ids: they pair by 1-based
// ordinal, so an appended or truncated tail reports as added or removed,
// while a middle insertion shifts the ordinals after it — v2 plans, whose
// ids are required, are always paired exactly. A supersede link this window
// introduces overrides the pairing once: the retired step reports the
// contract change against its replacement, so a restated replacement is not
// material while a capability move is; later windows see the pair as two
// ordinary steps and stay quiet.
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
	// pairedWith names the retired steps this window's new links consumed,
	// so neither side of the pair double-reports below.
	pairedWith := make(map[string]bool)
	consumed := make(map[string]bool)
	for _, key := range nextKeys {
		item := nextByKey[key]
		if item.SupersededBy == "" {
			continue
		}
		before, ok := oldByKey[key]
		if !ok || before.SupersededBy == item.SupersededBy {
			continue
		}
		replacement, ok := nextByKey[item.SupersededBy]
		if !ok {
			continue
		}
		pairedWith[key] = true
		consumed[item.SupersededBy] = true
		diff = append(diff, stepFieldDiff(key, before, replacement)...)
	}
	for _, key := range oldKeys {
		if pairedWith[key] {
			continue
		}
		if _, ok := nextByKey[key]; !ok {
			diff = append(diff, PlanMaterialChange{Target: key, Field: "step", Change: MaterialRemoved})
		}
	}
	for _, key := range nextKeys {
		if consumed[key] || pairedWith[key] {
			continue
		}
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
	if old.Model != next.Model {
		diff = append(diff, PlanMaterialChange{
			Target: key,
			Field:  "model",
			Change: MaterialChanged,
			Detail: fmt.Sprintf("%s to %s", old.Model, next.Model),
		})
	}
	diff = append(diff, actionsDiff(key, old.Actions, next.Actions)...)
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
