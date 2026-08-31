package harnesssettings

import (
	"errors"
	"slices"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Draft is submitted to Apply. BaseToken scopes optimistic concurrency to the
// plan.defaults YAML section, so unrelated edits can merge automatically.
// TypeRenames carries explicit UI intent for current-plan migration; it is not
// persisted as configuration.
//
// Plan-policy mutations go through the methods below — they keep the plan
// compile-clean and the rename chain coalesced — so widgets editing a draft
// never splice the capability hierarchy by hand.
type Draft struct {
	BaseToken   string
	Plan        plangate.Defaults
	TypeRenames map[session.StepType]session.StepType

	// CompactReminderTokens mirrors Snapshot.Compaction.ReminderTokens; the
	// pane edits it directly — a plain int carries no hierarchy invariants.
	CompactReminderTokens int

	// AgentModels mirrors Snapshot.AgentModels: role → model name. The
	// Agents tab edits it directly; nil, missing, or empty entries mean
	// "inherit the session model" and are dropped at Apply.
	AgentModels map[string]string

	// openedNames are the step types present when the draft was created;
	// RecordRename records renames only for them, because types created
	// inside this draft cannot carry current-plan references.
	openedNames map[session.StepType]struct{}
}

// AssignmentRank reports the index of the least capable step type the tool is
// assigned to, or -1 when the tool is unassigned.
func (d *Draft) AssignmentRank(tool string) int {
	for i, typ := range d.Plan.Types {
		if slices.Contains(typ.Tools, tool) {
			return i
		}
	}
	return -1
}

// TogglePermission handles a click on one type's tool row. The row shows
// checked while the tool's least capable assignment sits at or below that
// type: clicking a checked row forbids the tool from that type onward (the
// assignment moves one type up, or clears when no more capable type remains),
// and clicking an unchecked row assigns the tool starting at that type. The
// exemption list always loses the tool — the two states are exclusive.
func (d *Draft) TogglePermission(typeIndex int, tool string) {
	if typeIndex < 0 || typeIndex >= len(d.Plan.Types) {
		return
	}
	minimum := d.AssignmentRank(tool)
	allowed := minimum >= 0 && minimum <= typeIndex
	d.removeToolAssignments(tool)
	d.Plan.AdditionalExemptions = deleteString(d.Plan.AdditionalExemptions, tool)
	if allowed {
		if typeIndex+1 < len(d.Plan.Types) {
			d.Plan.Types[typeIndex+1].Tools = append(d.Plan.Types[typeIndex+1].Tools, tool)
		}
	} else {
		d.Plan.Types[typeIndex].Tools = append(d.Plan.Types[typeIndex].Tools, tool)
	}
}

// ToggleOutsidePlan flips the tool's plan-gate exemption, dropping any type
// assignment first — the two states are exclusive.
func (d *Draft) ToggleOutsidePlan(tool string) {
	if slices.Contains(d.Plan.AdditionalExemptions, tool) {
		d.Plan.AdditionalExemptions = deleteString(d.Plan.AdditionalExemptions, tool)
	} else {
		d.removeToolAssignments(tool)
		d.Plan.AdditionalExemptions = append(d.Plan.AdditionalExemptions, tool)
	}
}

// AddType appends a new step type, keeping the plan compile-clean.
func (d *Draft) AddType(name session.StepType) error {
	candidate := d.compiled()
	candidate.Types = append(candidate.Types, plangate.TypeDefaults{Name: name})
	return d.commit(candidate)
}

// RenameType renames the type at index, keeping the plan compile-clean, and
// reports the previous name so callers can record migration intent.
func (d *Draft) RenameType(index int, name session.StepType) (session.StepType, error) {
	if index < 0 || index >= len(d.Plan.Types) {
		return "", errors.New("selected step type no longer exists")
	}
	old := d.Plan.Types[index].Name
	candidate := d.compiled()
	candidate.Types[index].Name = name
	if err := d.commit(candidate); err != nil {
		return "", err
	}
	return old, nil
}

// DeleteType removes the type at index and drops rename entries naming it.
func (d *Draft) DeleteType(index int) {
	if index < 0 || index >= len(d.Plan.Types) {
		return
	}
	name := d.Plan.Types[index].Name
	d.Plan.Types = slices.Delete(d.Plan.Types, index, index+1)
	for from, to := range d.TypeRenames {
		if from == name || to == name {
			delete(d.TypeRenames, from)
		}
	}
}

// MoveType swaps the type with its neighbor and reports whether it moved.
// Reordering changes only cascade semantics — plan references and renames are
// untouched, so a type in use may still move.
func (d *Draft) MoveType(index, delta int) bool {
	target := index + delta
	if index < 0 || index >= len(d.Plan.Types) || target < 0 || target >= len(d.Plan.Types) {
		return false
	}
	d.Plan.Types[index], d.Plan.Types[target] = d.Plan.Types[target], d.Plan.Types[index]
	return true
}

// AddPlanAction appends a plan-scope default action, keeping the draft
// compile-clean; new plans inherit it when their author defines none.
func (d *Draft) AddPlanAction() error {
	candidate := d.compiled()
	candidate.Actions = append(candidate.Actions, session.PlanAction{
		Event: session.PlanActionOnPlanStart,
		Type:  session.PlanActionCompact,
	})
	return d.commit(candidate)
}

// RemovePlanAction drops the plan-scope default action at index.
func (d *Draft) RemovePlanAction(index int) {
	if index < 0 || index >= len(d.Plan.Actions) {
		return
	}
	d.Plan.Actions = slices.Delete(d.Plan.Actions, index, index+1)
}

// SetAuthoringPolicy selects the plan-mode authoring grammar, keeping the
// draft compile-clean: a value outside plangate's closed enum is refused
// here, in the pane, rather than at Apply.
func (d *Draft) SetAuthoringPolicy(value plangate.AuthoringPolicy) error {
	candidate := d.compiled()
	candidate.AuthoringPolicy = value
	return d.commit(candidate)
}

// AddTypeAction appends a step-scope default action to the type at index.
func (d *Draft) AddTypeAction(typeIndex int) error {
	if typeIndex < 0 || typeIndex >= len(d.Plan.Types) {
		return errors.New("selected step type no longer exists")
	}
	candidate := d.compiled()
	candidate.Types[typeIndex].Actions = append(candidate.Types[typeIndex].Actions, session.PlanAction{
		Event: session.PlanActionOnStepStart,
		Type:  session.PlanActionCompact,
	})
	return d.commit(candidate)
}

// RemoveTypeAction drops the step-scope default action at index from the
// type at typeIndex.
func (d *Draft) RemoveTypeAction(typeIndex, actionIndex int) {
	if typeIndex < 0 || typeIndex >= len(d.Plan.Types) {
		return
	}
	actions := d.Plan.Types[typeIndex].Actions
	if actionIndex < 0 || actionIndex >= len(actions) {
		return
	}
	d.Plan.Types[typeIndex].Actions = slices.Delete(actions, actionIndex, actionIndex+1)
}

// Reset restores the built-in defaults and drops all rename intent.
func (d *Draft) Reset() {
	d.Plan = plangate.DefaultDefaults()
	d.TypeRenames = nil
}

// RecordRename coalesces rename intent for migration: renaming a→b and then
// b→c records a→c. Only types that existed when the draft was opened are
// recorded — types created inside this draft carry no plan references.
func (d *Draft) RecordRename(old, name session.StepType) {
	source := old
	for from, to := range d.TypeRenames {
		if to == old {
			source = from
			delete(d.TypeRenames, from)
			break
		}
	}
	if source == name {
		return
	}
	if _, existedWhenOpened := d.openedNames[source]; !existedWhenOpened {
		return
	}
	if d.TypeRenames == nil {
		d.TypeRenames = make(map[session.StepType]session.StepType)
	}
	d.TypeRenames[source] = name
}

// compiled re-canonicalizes the current plan through the policy compiler;
// input that cannot compile (a broken base) is returned unchanged and fails
// at commit.
func (d *Draft) compiled() plangate.Defaults {
	policy, err := plangate.Compile(d.Plan)
	if err != nil {
		return d.Plan
	}
	return policy.Defaults()
}

func (d *Draft) commit(candidate plangate.Defaults) error {
	validated, err := plangate.Compile(candidate)
	if err != nil {
		return err
	}
	d.Plan = validated.Defaults()
	return nil
}

func (d *Draft) removeToolAssignments(tool string) {
	for i := range d.Plan.Types {
		d.Plan.Types[i].Tools = deleteString(d.Plan.Types[i].Tools, tool)
	}
}

func deleteString(values []string, value string) []string {
	return slices.DeleteFunc(values, func(item string) bool { return item == value })
}
