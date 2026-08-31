package planedit

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session"
)

// maxReportedConflicts bounds the conflict list rendered into the one-line
// error strip; the rest is summarized as a count.
const maxReportedConflicts = 3

// rebase moves a draft built on old onto fresh, keeping every edit the newer
// revision does not contradict. One rule decides every field: a field the
// editor never touched takes the newer value, a field only the editor touched
// keeps its edit, and a field both changed keeps the newer value and is
// reported. The returned list is exactly what the merge took away, so the
// modal can name it instead of asking for the whole draft back.
func (d Draft) rebase(old, fresh session.Plan) (Draft, []string) {
	var conflicts []string
	out := Draft{
		Goal:     rebaseText("goal", d.Goal, old.Goal, fresh.Goal, &conflicts),
		Approach: rebaseText("approach", d.Approach, old.Approach, fresh.Approach, &conflicts),
		WorkingContext: rebaseText(
			"working context", d.WorkingContext, old.WorkingContext, fresh.WorkingContext, &conflicts,
		),
		ModelsByType: rebaseModels(d.ModelsByType, old.ModelsByType, fresh.ModelsByType, &conflicts),
	}
	out.SuccessCriteria = rebaseDirectives(d.SuccessCriteria, fresh.SuccessCriteria, "criterion", &conflicts)
	out.Constraints = rebaseDirectives(d.Constraints, fresh.Constraints, "constraint", &conflicts)
	out.Steps = rebaseSteps(d.Steps, old, fresh, &conflicts)
	return out, conflicts
}

// rebaseText is the three-way merge of one scalar.
func rebaseText(label, draft, old, fresh string, conflicts *[]string) string {
	switch {
	case draft == old:
		return fresh
	case fresh == old || fresh == draft:
		return draft
	default:
		*conflicts = append(*conflicts, label)
		return fresh
	}
}

func rebaseModels(draft, old, fresh map[session.StepType]string, conflicts *[]string) map[session.StepType]string {
	types := make(map[session.StepType]bool, len(draft)+len(fresh))
	for typ := range draft {
		types[typ] = true
	}
	for typ := range old {
		types[typ] = true
	}
	for typ := range fresh {
		types[typ] = true
	}
	var out map[session.StepType]string
	for _, typ := range slices.Sorted(maps.Keys(types)) {
		value := rebaseText("model for "+stepTypeLabel(typ), draft[typ], old[typ], fresh[typ], conflicts)
		if value == "" {
			continue
		}
		if out == nil {
			out = make(map[session.StepType]string)
		}
		out[typ] = value
	}
	return out
}

// rebaseDirectives keys entries by the durable value they were drawn from,
// because that value is the only identity a directive has. A directive the
// newer revision no longer carries cannot be updated, so the entry goes and
// only an edited one is reported as lost.
func rebaseDirectives(draft []directiveDraft, fresh []string, label string, conflicts *[]string) []directiveDraft {
	present := make(map[string]bool, len(fresh))
	for _, value := range fresh {
		present[value] = true
	}
	claimed := make(map[string]bool, len(draft)+len(fresh))
	out := make([]directiveDraft, 0, len(draft)+len(fresh))
	for _, entry := range draft {
		value := strings.TrimSpace(entry.Value)
		if entry.New {
			// An addition the newer revision already carries is no longer an
			// addition: it becomes an ordinary entry so no add op is emitted.
			if !present[value] {
				out = append(out, entry)
				continue
			}
			if !claimed[value] {
				claimed[value] = true
				out = append(out, directiveDraft{Value: value, Original: value})
			}
			continue
		}
		if !present[entry.Original] {
			if value != entry.Original {
				*conflicts = append(*conflicts, fmt.Sprintf("%s %q", label, entry.Original))
			}
			continue
		}
		if claimed[entry.Original] {
			continue
		}
		claimed[entry.Original] = true
		out = append(out, entry)
	}
	for _, value := range fresh {
		if claimed[value] {
			continue
		}
		claimed[value] = true
		out = append(out, directiveDraft{Value: value, Original: value})
	}
	return revertBlockedRenames(out, label, conflicts)
}

// revertBlockedRenames cancels a rename onto a value the rebased list still
// holds, because the durable list has no room for the duplicate. A value another
// entry renames away is not in the way, so a swap and a chain of renames survive
// the merge. Canceling a rename puts its own value back, which can block a
// second one, so the pass repeats until the list settles.
func revertBlockedRenames(entries []directiveDraft, label string, conflicts *[]string) []directiveDraft {
	for settled := false; !settled; {
		settled = true
		held := make(map[string]bool, len(entries))
		for _, entry := range entries {
			if value := strings.TrimSpace(entry.Value); !entry.New && value == entry.Original {
				held[value] = true
			}
		}
		for i, entry := range entries {
			value := strings.TrimSpace(entry.Value)
			if entry.New || value == entry.Original || !held[value] {
				continue
			}
			*conflicts = append(*conflicts, fmt.Sprintf("%s %q", label, entry.Original))
			entries[i].Value = entry.Original
			settled = false
		}
	}
	return entries
}

// rebaseSteps keys steps by id and re-points every base index at the fresh
// plan, because the ops compiler diffs a step against base.Items[baseIndex].
func rebaseSteps(draft []DraftStep, old, fresh session.Plan, conflicts *[]string) []DraftStep {
	freshIndex := itemIndex(fresh.Items)
	oldIndex := itemIndex(old.Items)

	kept := make(map[string]bool, len(draft))
	out := make([]DraftStep, 0, len(draft)+len(fresh.Items))
	for _, step := range draft {
		if step.isNew {
			idx, taken := freshIndex[step.ID]
			if !taken {
				out = append(out, step)
				continue
			}
			// The newer revision took this id. Inserting it again would be
			// refused, so the new step becomes an edit of the step that took it.
			*conflicts = append(*conflicts, fmt.Sprintf("step %q was added to the plan", step.ID))
			step.isNew = false
			kept[step.ID] = true
			out = append(out, rebaseStep(step, fresh.Items[idx], fresh.Items[idx], idx, conflicts))
			continue
		}
		idx, alive := freshIndex[step.baseID]
		if !alive {
			oldIdx, existed := oldIndex[step.baseID]
			if existed && stepEdited(step, old.Items[oldIdx]) {
				*conflicts = append(*conflicts, fmt.Sprintf("step %q was removed from the plan", step.baseID))
			}
			continue
		}
		kept[step.baseID] = true
		// A step with no counterpart in the old base entered the draft from a
		// later revision already; there is nothing to merge against, so its
		// current values stand.
		base := fresh.Items[idx]
		if oldIdx, existed := oldIndex[step.baseID]; existed {
			base = old.Items[oldIdx]
		}
		out = append(out, rebaseStep(step, base, fresh.Items[idx], idx, conflicts))
	}

	for i, item := range fresh.Items {
		if item.ID == "" || kept[item.ID] {
			continue
		}
		if _, existed := oldIndex[item.ID]; existed {
			// The editor deleted it. Only a pending step can be deleted, so a
			// step that has started cancels the deletion.
			if item.Status == session.PlanPending {
				continue
			}
			*conflicts = append(
				*conflicts,
				fmt.Sprintf("step %q is %s and can no longer be deleted", item.ID, item.Status),
			)
		}
		out = slices.Insert(out, spliceIndex(out, fresh.Items[:i]), draftStep(item, i))
	}
	return out
}

// rebaseStep merges one surviving step. Lifecycle facts are never merged: they
// belong to the plan, and the editor only ever displayed them.
func rebaseStep(step DraftStep, old, fresh session.PlanItem, index int, conflicts *[]string) DraftStep {
	var fields []string
	step.baseID, step.baseIndex = fresh.ID, index
	step.ID, step.Status, step.Type, step.JIT = fresh.ID, fresh.Status, fresh.Type, fresh.JIT
	step.Content = rebaseText("content", step.Content, old.Content, fresh.Content, &fields)
	step.Why = rebaseText("why", step.Why, old.Why, fresh.Why, &fields)
	step.DoneWhen = rebaseText("done when", step.DoneWhen, old.DoneWhen, fresh.DoneWhen, &fields)
	step.Note = rebaseText("note", step.Note, old.Note, fresh.Note, &fields)
	step.Risk = rebaseText("risk", step.Risk, old.Risk, fresh.Risk, &fields)
	step.Model = rebaseText("model", step.Model, old.Model, fresh.Model, &fields)
	switch {
	case actionsEqual(step.Actions, old.Actions):
		step.Actions = slices.Clone(fresh.Actions)
	case actionsEqual(fresh.Actions, old.Actions) || actionsEqual(step.Actions, fresh.Actions):
	default:
		fields = append(fields, "actions")
		step.Actions = slices.Clone(fresh.Actions)
	}
	if len(fields) > 0 {
		*conflicts = append(*conflicts, fmt.Sprintf("step %q: %s", fresh.ID, strings.Join(fields, ", ")))
	}
	return step
}

// spliceIndex places a step the newer revision added where its fresh
// neighbors put it, so an inserted step does not jump to the end of a list
// the editor has reordered.
func spliceIndex(out []DraftStep, before []session.PlanItem) int {
	for _, item := range slices.Backward(before) {
		if item.ID == "" {
			continue
		}
		for i, step := range out {
			if step.baseID == item.ID {
				return i + 1
			}
		}
	}
	return 0
}

func itemIndex(items []session.PlanItem) map[string]int {
	index := make(map[string]int, len(items))
	for i, item := range items {
		if item.ID != "" {
			index[item.ID] = i
		}
	}
	return index
}

// stepEdited reports whether the editor changed anything the patch path would
// carry; lifecycle fields are excluded because the editor cannot author them.
func stepEdited(step DraftStep, item session.PlanItem) bool {
	return step.Content != item.Content || step.Why != item.Why || step.DoneWhen != item.DoneWhen ||
		step.Note != item.Note || step.Risk != item.Risk || step.Model != item.Model ||
		!actionsEqual(step.Actions, item.Actions)
}

func actionsEqual(a, b []session.PlanAction) bool {
	return slices.EqualFunc(authoredActions(a), authoredActions(b), session.PlanActionEqual)
}

// conflictMessage names what the merge dropped, bounded to one strip line.
func conflictMessage(revision uint64, conflicts []string) string {
	shown := conflicts
	suffix := ""
	if len(shown) > maxReportedConflicts {
		shown, suffix = shown[:maxReportedConflicts], fmt.Sprintf(" (+%d more)", len(conflicts)-maxReportedConflicts)
	}
	return fmt.Sprintf(
		"plan moved to rev %d; draft rebased, newer values kept for: %s%s; review and press ctrl+s again",
		revision, strings.Join(shown, ", "), suffix,
	)
}
