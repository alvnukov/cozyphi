package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// maxPlanPatchOps bounds one batch: a patch is a reconciliation of one observed
// revision, not a streaming editor.
const maxPlanPatchOps = 32

// StalePlanRevisionError reports a compare-and-set refusal: the patch was built
// against a revision the plan has already left behind. It carries both numbers
// so a caller able to rebase its own edits does not have to parse the message.
type StalePlanRevisionError struct {
	Expected uint64
	Actual   uint64
}

func (e *StalePlanRevisionError) Error() string {
	return fmt.Sprintf(
		"session: plan revision is %d; patch expected %d; re-fetch with action get before patching",
		e.Actual, e.Expected,
	)
}

// Patch op names. Each names one domain-specific mutation; the set is the
// whole patch vocabulary.
const (
	PlanPatchSetPlanFields    = "set_plan_fields"
	PlanPatchReplaceContext   = "replace_context"
	PlanPatchUpdateStep       = "update_step"
	PlanPatchInsertStep       = "insert_step"
	PlanPatchRemoveStep       = "remove_step"
	PlanPatchSupersedeStep    = "supersede_step"
	PlanPatchReorderSteps     = "reorder_steps"
	PlanPatchAddConstraint    = "add_constraint"
	PlanPatchUpdateConstraint = "update_constraint"
	PlanPatchRemoveConstraint = "remove_constraint"
	PlanPatchAddCriterion     = "add_criterion"
	PlanPatchUpdateCriterion  = "update_criterion"
	PlanPatchRemoveCriterion  = "remove_criterion"
)

var planPatchOpNames = []string{
	PlanPatchSetPlanFields,
	PlanPatchReplaceContext,
	PlanPatchUpdateStep,
	PlanPatchInsertStep,
	PlanPatchRemoveStep,
	PlanPatchSupersedeStep,
	PlanPatchReorderSteps,
	PlanPatchAddConstraint,
	PlanPatchUpdateConstraint,
	PlanPatchRemoveConstraint,
	PlanPatchAddCriterion,
	PlanPatchUpdateCriterion,
	PlanPatchRemoveCriterion,
}

// PatchValue is one scalar slot in a patch operation: an absent field leaves
// the stored value unchanged, a value replaces it, and an explicit null clears
// it. Only optional fields accept a clear; owners of required fields reject a
// null (or an empty) value at apply time.
type PatchValue[T any] struct {
	Set   bool
	Value T
}

// UnmarshalJSON distinguishes absent (never called) from null (called with
// "null"), which a plain pointer cannot. Ops are decode-only on purpose:
// nothing marshals a PlanPatchOp back out, and a naive marshal of an unset
// slot would read back as an explicit clear.
func (p *PatchValue[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		p.Set, p.Value = true, *new(T)
		return nil
	}
	p.Set = true
	return json.Unmarshal(data, &p.Value)
}

// PlanPatchOp is one domain-specific plan mutation addressed by stable
// identity — a step id, a directive's exact text — never by array position.
// Which fields each op reads is part of the op's contract; a populated field
// the op does not take is rejected instead of silently dropped.
type PlanPatchOp struct {
	Op string `json:"op"`

	// set_plan_fields
	Goal     PatchValue[string] `json:"goal,omitempty"`
	Approach PatchValue[string] `json:"approach,omitempty"`

	// Automation fields. Actions replaces a whole action list: step-level
	// on update_step, plan-level on set_plan_fields; null clears it. Model
	// is the step's model override ("" follows the type map); modelsByType
	// replaces the per-step-type model map, null clears it.
	Actions      PatchValue[[]PlanAction]        `json:"actions,omitempty"`
	Model        PatchValue[string]              `json:"model,omitempty"`
	ModelsByType PatchValue[map[StepType]string] `json:"modelsByType,omitempty"`

	// replace_context (the whole working context; there is no append)
	WorkingContext PatchValue[string] `json:"workingContext,omitempty"`

	// update_step. Skills is the narrow model path into step automation: the
	// value re-authors the injected skill list (null or [] removes the
	// injection); Actions stays the human-owned whole-list replace.
	ID       string               `json:"id,omitempty"`
	Content  PatchValue[string]   `json:"content,omitempty"`
	Why      PatchValue[string]   `json:"why,omitempty"`
	DoneWhen PatchValue[string]   `json:"doneWhen,omitempty"`
	Risk     PatchValue[string]   `json:"risk,omitempty"`
	Note     PatchValue[string]   `json:"note,omitempty"`
	Skills   PatchValue[[]string] `json:"skills,omitempty"`

	// insert_step
	Before string    `json:"before,omitempty"`
	After  string    `json:"after,omitempty"`
	Step   *PlanItem `json:"step,omitempty"`

	// reorder_steps (the complete new order of every step id)
	IDs []string `json:"ids,omitempty"`

	// directive add/remove (value) and update (from, to)
	Value string `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// PlanPatchSummary is the compact delta a successful patch answers with: what
// changed, never the whole snapshot. Diff is the material subset — the same
// table that decides approval — so the receipt states exactly why approval
// was kept or revoked.
type PlanPatchSummary struct {
	PlanFields      []string             `json:"planFields,omitempty"`
	StepsUpdated    []string             `json:"stepsUpdated,omitempty"`
	StepsInserted   []string             `json:"stepsInserted,omitempty"`
	StepsRemoved    []string             `json:"stepsRemoved,omitempty"`
	StepsSuperseded []string             `json:"stepsSuperseded,omitempty"`
	StepsReordered  bool                 `json:"stepsReordered,omitempty"`
	Diff            []PlanMaterialChange `json:"diff,omitempty"`
}

func (s *PlanPatchSummary) addPlanField(field string) {
	if !slices.Contains(s.PlanFields, field) {
		s.PlanFields = append(s.PlanFields, field)
	}
}

func (s *PlanPatchSummary) addStepUpdated(id string) {
	if !slices.Contains(s.StepsUpdated, id) {
		s.StepsUpdated = append(s.StepsUpdated, id)
	}
}

// PatchPlan atomically applies a batch of domain-specific operations to the
// current v2 plan. The whole batch is all-or-none: every operation is applied
// to an in-memory candidate, any failure abandons it, and only a fully valid
// result reaches commitPlanLocked — so a failing batch leaves the durable
// plan, its revision, and its approval untouched. expectedRevision guards the
// get→patch round trip against lost updates; a stale expectation reports the
// actual revision. Step status, approval, evidence, and audit metadata are not
// patchable: they belong to the lifecycle transitions and the user.
func (sm *Manager) PatchPlan(
	expectedRevision uint64,
	ops []PlanPatchOp,
	autoApprove bool,
) (Plan, PlanPatchSummary, error) {
	if sm == nil {
		return Plan{}, PlanPatchSummary{}, errors.New("session: plan manager is nil")
	}
	if len(ops) == 0 {
		return Plan{}, PlanPatchSummary{}, errors.New("session: plan patch has no operations")
	}
	if len(ops) > maxPlanPatchOps {
		return Plan{}, PlanPatchSummary{}, fmt.Errorf(
			"session: plan patch has %d operations; maximum is %d", len(ops), maxPlanPatchOps,
		)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.plan.Schema.IsV2() {
		return Plan{}, PlanPatchSummary{}, errors.New(
			"session: plan patch requires a v2 plan; send the full contract with action create",
		)
	}
	if expectedRevision != sm.plan.Revision {
		// The refusal is the friction event; the retry is the next call.
		sm.telemetry.PatchRetry()
		return Plan{}, PlanPatchSummary{}, &StalePlanRevisionError{
			Expected: expectedRevision,
			Actual:   sm.plan.Revision,
		}
	}

	candidate := sm.plan.Clone()
	var summary PlanPatchSummary
	for i, op := range ops {
		if err := applyPlanPatchOp(&candidate, op, &summary); err != nil {
			return Plan{}, PlanPatchSummary{}, fmt.Errorf("session: patch op %d (%s): %w", i+1, op.Op, err)
		}
		// Re-validate after every operation so a bound or contract violation
		// names the operation that caused it, not just the batch.
		checked, err := revalidatePatchedPlan(candidate)
		if err != nil {
			return Plan{}, PlanPatchSummary{}, fmt.Errorf("session: patch op %d (%s): %w", i+1, op.Op, err)
		}
		candidate = checked
	}

	// The patch path owns the serialized budget exactly like authoring: rune
	// caps alone do not bound bytes once wide runes multiply.
	if err := planWithinSerializedBudget(candidate); err != nil {
		return Plan{}, PlanPatchSummary{}, err
	}

	plan, diff, err := sm.commitPlanLocked(candidate, autoApprove)
	if err != nil {
		return Plan{}, PlanPatchSummary{}, err
	}
	summary.Diff = diff
	return plan, summary, nil
}

// revalidatePatchedPlan runs the patched candidate through the one v2
// normalize path, then restores the commit-stamped fields normalizePlanV2
// does not carry. Bounds, requireds, id slugs, and uniqueness all stay owned
// by that single table.
func revalidatePatchedPlan(plan Plan) (Plan, error) {
	checked, err := normalizePlanV2(PlanV2{
		Goal:            plan.Goal,
		Approach:        plan.Approach,
		SuccessCriteria: plan.SuccessCriteria,
		Constraints:     plan.Constraints,
		WorkingContext:  plan.WorkingContext,
		Actions:         plan.Actions,
		ModelsByType:    plan.ModelsByType,
		Items:           plan.Items,
		Result:          plan.Result,
		ClosedAt:        plan.ClosedAt,
	})
	if err != nil {
		return Plan{}, err
	}
	checked.Revision = plan.Revision
	checked.UpdatedAt = plan.UpdatedAt
	checked.Approved = plan.Approved
	// normalizePlanV2 strips attempts with the rest of the create-only
	// input; harness-recorded evidence is restored like the audit ledger,
	// in item order — normalize never reorders steps. Action run history
	// comes back the same way, but only onto definitions that survived the
	// patch: a re-authored action list starts with an empty history.
	for i := range checked.Items {
		checked.Items[i].Attempts = append([]PlanAttempt(nil), plan.Items[i].Attempts...)
		restorePlanActionRuns(&checked.Items[i].Actions, plan.Items[i].Actions)
	}
	restorePlanActionRuns(&checked.Actions, plan.Actions)
	checked.Events = plan.Events
	checked.Mutations = plan.Mutations
	// User-owned just-in-time grants and the epoch they hang on are harness
	// state like the audit ledger: the create-only normalize path drops
	// them, so they are restored like the ledger, in place.
	checked.ContractEpoch = plan.ContractEpoch
	checked.JITApprovals = plan.JITApprovals
	return checked, nil
}

// applyPlanPatchOp mutates the candidate plan in place and records what
// changed. Every error names the offending id or field; the caller adds the
// operation index.
func applyPlanPatchOp(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	switch op.Op {
	case PlanPatchSetPlanFields:
		if err := op.rejectForeignFields("goal", "approach", "actions", "modelsByType"); err != nil {
			return err
		}
		return applySetPlanFields(plan, op, summary)
	case PlanPatchReplaceContext:
		if err := op.rejectForeignFields("workingContext"); err != nil {
			return err
		}
		if !op.WorkingContext.Set {
			return errors.New("sets no fields")
		}
		plan.WorkingContext = strings.TrimSpace(op.WorkingContext.Value)
		summary.addPlanField("workingContext")
		return nil
	case PlanPatchUpdateStep:
		if err := op.rejectForeignFields(
			"id",
			"content",
			"why",
			"doneWhen",
			"risk",
			"note",
			"actions",
			"model",
			"skills",
		); err != nil {
			return err
		}
		return applyUpdateStep(plan, op, summary)
	case PlanPatchInsertStep:
		if err := op.rejectForeignFields("before", "after", "step"); err != nil {
			return err
		}
		return applyInsertStep(plan, op, summary)
	case PlanPatchRemoveStep:
		if err := op.rejectForeignFields("id"); err != nil {
			return err
		}
		return applyRemoveStep(plan, op, summary)
	case PlanPatchSupersedeStep:
		if err := op.rejectForeignFields("id", "step"); err != nil {
			return err
		}
		return applySupersedeStep(plan, op, summary)
	case PlanPatchReorderSteps:
		if err := op.rejectForeignFields("ids"); err != nil {
			return err
		}
		return applyReorderSteps(plan, op, summary)
	case PlanPatchAddConstraint, PlanPatchUpdateConstraint, PlanPatchRemoveConstraint,
		PlanPatchAddCriterion, PlanPatchUpdateCriterion, PlanPatchRemoveCriterion:
		spec, ok := directiveSpecOf(op.Op)
		if !ok {
			return fmt.Errorf("unknown op %q (use %s)", op.Op, strings.Join(planPatchOpNames, ", "))
		}
		return applyDirectiveOp(plan, op, spec, summary)
	default:
		return fmt.Errorf("unknown op %q (use %s)", op.Op, strings.Join(planPatchOpNames, ", "))
	}
}

// applySetPlanFields replaces required plan prose. Null on a required field is
// refused here rather than by the late normalize pass, so the error names the
// field the operation tried to clear.
func applySetPlanFields(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	if !op.Goal.Set && !op.Approach.Set && !op.Actions.Set && !op.ModelsByType.Set {
		return errors.New("sets no fields")
	}
	if op.Goal.Set {
		goal := strings.TrimSpace(op.Goal.Value)
		if goal == "" {
			return errors.New("goal cannot be cleared; it is required")
		}
		plan.Goal = goal
		summary.addPlanField("goal")
	}
	if op.Approach.Set {
		approach := strings.TrimSpace(op.Approach.Value)
		if approach == "" {
			return errors.New("approach cannot be cleared; it is required")
		}
		plan.Approach = approach
		summary.addPlanField("approach")
	}
	if op.Actions.Set {
		// The whole plan-level list goes at once: actions are a small set of
		// per-event hooks, and per-action patch ops would outgrow their use.
		plan.Actions = op.Actions.Value
		summary.addPlanField("actions")
	}
	if op.ModelsByType.Set {
		plan.ModelsByType = op.ModelsByType.Value
		summary.addPlanField("modelsByType")
	}
	return nil
}

// applyUpdateStep patches one step's contract prose and operational note.
// Status, evidence, and identity are not patchable; the required prose fields
// refuse a clear with the field named.
func applyUpdateStep(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	id := strings.TrimSpace(op.ID)
	if id == "" {
		return errors.New("step id is required")
	}
	idx := findStepByID(plan.Items, id)
	if idx < 0 {
		return fmt.Errorf("step %q not found", id)
	}
	if !op.Content.Set && !op.Why.Set && !op.DoneWhen.Set && !op.Risk.Set && !op.Note.Set &&
		!op.Actions.Set && !op.Model.Set && !op.Skills.Set {
		return fmt.Errorf("step %q sets no fields", id)
	}
	if op.Actions.Set {
		// Replacing the list is re-authoring: run history dies with the old
		// definitions (restorePlanActionRuns keeps it only when definitions
		// survive).
		plan.Items[idx].Actions = op.Actions.Value
	}
	if op.Skills.Set {
		// The value re-authors the injected list through the same fold the
		// authoring path uses; explicit null clears, like the empty list.
		skills := op.Skills.Value
		if skills == nil {
			skills = []string{}
		}
		item := &plan.Items[idx]
		item.Skills = skills
		compileStepSkills(item)
	}
	clears := []struct {
		slot       PatchValue[string]
		field      string
		assign     func(string)
		clearIsSet bool
	}{
		{slot: op.Content, field: "content", assign: func(v string) { plan.Items[idx].Content = v }},
		{slot: op.Why, field: "why", assign: func(v string) { plan.Items[idx].Why = v }},
		{slot: op.DoneWhen, field: "done_when", assign: func(v string) { plan.Items[idx].DoneWhen = v }},
		{slot: op.Risk, field: "risk", assign: func(v string) { plan.Items[idx].Risk = v }, clearIsSet: true},
		{slot: op.Note, field: "note", assign: func(v string) { plan.Items[idx].Note = v }, clearIsSet: true},
		{slot: op.Model, field: "model", assign: func(v string) { plan.Items[idx].Model = v }, clearIsSet: true},
	}
	for _, c := range clears {
		if !c.slot.Set {
			continue
		}
		value := strings.TrimSpace(c.slot.Value)
		if value == "" && !c.clearIsSet {
			return fmt.Errorf("step %q %s cannot be cleared; it is required", id, c.field)
		}
		c.assign(value)
	}
	summary.addStepUpdated(id)
	return nil
}

// freshStep strips the operational record a patch never inherits: a step
// entering the plan through insert or supersede starts pending with an
// empty history, exactly like a fresh create.
func freshStep(step PlanItem) PlanItem {
	step.Status = PlanPending
	step.Note = ""
	step.Evidence = ""
	step.Outcome = ""
	step.EvidenceRefs = nil
	step.Blocker = ""
	step.ResumeWhen = ""
	step.SupersededBy = ""
	for i := range step.Actions {
		step.Actions[i].Runs = nil
	}
	step.Attempts = nil
	return step
}

// applyInsertStep adds one new pending step next to an existing anchor. The
// step arrives with contract fields only; status starts pending and
// operational metadata starts empty, exactly like a fresh create.
func applyInsertStep(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	if op.Step == nil {
		return errors.New("step is required")
	}
	if op.Before != "" && op.After != "" {
		return errors.New("takes one anchor: before or after, not both")
	}
	anchor := strings.TrimSpace(op.Before)
	if anchor == "" {
		anchor = strings.TrimSpace(op.After)
	}
	if anchor == "" {
		return errors.New("before or after anchor is required")
	}
	idx := findStepByID(plan.Items, anchor)
	if idx < 0 {
		return fmt.Errorf("step %q not found", anchor)
	}
	item := freshStep(*op.Step)
	at := idx
	if op.After != "" {
		at = idx + 1
	}
	plan.Items = slices.Insert(plan.Items, at, item)
	summary.StepsInserted = append(summary.StepsInserted, item.ID)
	return nil
}

// applyRemoveStep drops a step that never started: only pending steps can be
// removed, because any other status already has history worth auditing.
func applyRemoveStep(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	id := strings.TrimSpace(op.ID)
	if id == "" {
		return errors.New("step id is required")
	}
	idx := findStepByID(plan.Items, id)
	if idx < 0 {
		return fmt.Errorf("step %q not found", id)
	}
	if plan.Items[idx].Status != PlanPending {
		return fmt.Errorf("step %q is %s; only pending steps can be removed", id, plan.Items[idx].Status)
	}
	plan.Items = slices.Delete(plan.Items, idx, idx+1)
	summary.StepsRemoved = append(summary.StepsRemoved, id)
	return nil
}

// applySupersedeStep retires one step and lands its replacement in the same
// write: the old step turns superseded — terminal, its evidence, outcome, and
// attempts untouched — and a fresh pending step with the caller's new
// contract takes its place in line. It exists because a mid-plan capability
// change cannot use cancel (a cancellation poisons a success close) or
// remove_step (only pending steps may be removed, on purpose): the work
// happened and stays audited, while the obligation moves to the replacement.
func applySupersedeStep(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	if op.Step == nil {
		return errors.New("step is required")
	}
	id := strings.TrimSpace(op.ID)
	if id == "" {
		return errors.New("step id is required")
	}
	idx := findStepByID(plan.Items, id)
	if idx < 0 {
		return fmt.Errorf("step %q not found", id)
	}
	if plan.Items[idx].Status == PlanSuperseded {
		return fmt.Errorf("step %q is already superseded; supersede its replacement instead", id)
	}
	item := freshStep(*op.Step)
	plan.Items[idx].SupersededBy = item.ID
	plan.Items[idx].Status = PlanSuperseded
	plan.Items = slices.Insert(plan.Items, idx+1, item)
	summary.StepsSuperseded = append(summary.StepsSuperseded, id+"->"+item.ID)
	return nil
}

// applyReorderSteps rebuilds the step order from a complete list of ids. A
// partial or duplicate list is refused; a list naming the existing order is
// applied but reported as unchanged.
func applyReorderSteps(plan *Plan, op PlanPatchOp, summary *PlanPatchSummary) error {
	if len(op.IDs) == 0 {
		return errors.New("ids is required")
	}
	if len(op.IDs) != len(plan.Items) {
		return fmt.Errorf("reorder lists %d ids for %d steps", len(op.IDs), len(plan.Items))
	}
	reordered := make([]PlanItem, 0, len(plan.Items))
	seen := make(map[string]struct{}, len(op.IDs))
	for _, id := range op.IDs {
		idx := findStepByID(plan.Items, id)
		if idx < 0 {
			return fmt.Errorf("step %q not found", id)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("id %q listed twice", id)
		}
		seen[id] = struct{}{}
		reordered = append(reordered, plan.Items[idx])
	}
	if !sameStepOrder(plan.Items, reordered) {
		summary.StepsReordered = true
	}
	plan.Items = reordered
	return nil
}

// directiveSpec is everything one directive op needs, derived from the op
// name in a single place.
type directiveSpec struct {
	what      string // human name in errors: "constraint" / "success criterion"
	kind      string // "add" / "update" / "remove"
	criterion bool   // targets success criteria instead of constraints
	summaryAs string // planFields entry in the summary
}

func directiveSpecOf(op string) (directiveSpec, bool) {
	specs := map[string]directiveSpec{
		PlanPatchAddConstraint:    {what: "constraint", kind: "add", summaryAs: "constraints"},
		PlanPatchUpdateConstraint: {what: "constraint", kind: "update", summaryAs: "constraints"},
		PlanPatchRemoveConstraint: {what: "constraint", kind: "remove", summaryAs: "constraints"},
		PlanPatchAddCriterion: {
			what:      "success criterion",
			kind:      "add",
			criterion: true,
			summaryAs: "successCriteria",
		},
		PlanPatchUpdateCriterion: {
			what:      "success criterion",
			kind:      "update",
			criterion: true,
			summaryAs: "successCriteria",
		},
		PlanPatchRemoveCriterion: {
			what:      "success criterion",
			kind:      "remove",
			criterion: true,
			summaryAs: "successCriteria",
		},
	}
	spec, ok := specs[op]
	return spec, ok
}

// applyDirectiveOp adds, updates, or removes one constraint or success
// criterion. A directive's exact text is its identity: updates match by the
// current text, never by position.
func applyDirectiveOp(plan *Plan, op PlanPatchOp, spec directiveSpec, summary *PlanPatchSummary) error {
	allowed := []string{"value"}
	if spec.kind == "update" {
		allowed = []string{"from", "to"}
	}
	if foreign := opCheckForeign(op, allowed...); len(foreign) > 0 {
		return fmt.Errorf("%s takes no %s", op.Op, foreign[0])
	}

	list := &plan.Constraints
	if spec.criterion {
		list = &plan.SuccessCriteria
	}
	var (
		updated []string
		err     error
	)
	switch spec.kind {
	case "add":
		updated, err = addDirective(*list, op.Value, spec.what)
	case "update":
		updated, err = updateDirective(*list, op.From, op.To, spec.what)
	case "remove":
		updated, err = removeDirective(*list, op.Value, spec.what)
	}
	if err != nil {
		return err
	}
	*list = updated
	summary.addPlanField(spec.summaryAs)
	return nil
}

func addDirective(list []string, value, what string) ([]string, error) {
	entry := strings.TrimSpace(value)
	if entry == "" {
		return nil, fmt.Errorf("%s value is required", what)
	}
	if slices.Contains(list, entry) {
		return nil, fmt.Errorf("%s %q already exists", what, entry)
	}
	return append(slices.Clone(list), entry), nil
}

func updateDirective(list []string, from, to, what string) ([]string, error) {
	match := strings.TrimSpace(from)
	entry := strings.TrimSpace(to)
	if match == "" {
		return nil, fmt.Errorf("%s from is required", what)
	}
	if entry == "" {
		return nil, fmt.Errorf("%s to is required", what)
	}
	idx := slices.Index(list, match)
	if idx < 0 {
		return nil, fmt.Errorf("%s %q not found", what, match)
	}
	for i, existing := range list {
		if i != idx && existing == entry {
			return nil, fmt.Errorf("%s %q already exists", what, entry)
		}
	}
	updated := slices.Clone(list)
	updated[idx] = entry
	return updated, nil
}

func removeDirective(list []string, value, what string) ([]string, error) {
	entry := strings.TrimSpace(value)
	if entry == "" {
		return nil, fmt.Errorf("%s value is required", what)
	}
	idx := slices.Index(list, entry)
	if idx < 0 {
		return nil, fmt.Errorf("%s %q not found", what, entry)
	}
	return slices.Delete(slices.Clone(list), idx, idx+1), nil
}

// rejectForeignFields reports the first populated field the op does not
// accept, so a misrouted operation fails loudly instead of dropping work the
// caller believes it sent.
func (op PlanPatchOp) rejectForeignFields(allowed ...string) error {
	if foreign := opCheckForeign(op, allowed...); len(foreign) > 0 {
		return fmt.Errorf("%s takes no %s", op.Op, foreign[0])
	}
	return nil
}

// opCheckForeign collects every populated patch field outside the allowed
// set. The result is sorted so the reported offender is deterministic. The
// table is pinned to the struct's field list by TestPatchForeignTableCoversEveryField.
func opCheckForeign(op PlanPatchOp, allowed ...string) []string {
	populated := []struct {
		name string
		set  bool
	}{
		{"goal", op.Goal.Set},
		{"approach", op.Approach.Set},
		{"actions", op.Actions.Set},
		{"model", op.Model.Set},
		{"modelsByType", op.ModelsByType.Set},
		{"workingContext", op.WorkingContext.Set},
		{"id", op.ID != ""},
		{"content", op.Content.Set},
		{"why", op.Why.Set},
		{"doneWhen", op.DoneWhen.Set},
		{"risk", op.Risk.Set},
		{"note", op.Note.Set},
		{"skills", op.Skills.Set},
		{"before", op.Before != ""},
		{"after", op.After != ""},
		{"step", op.Step != nil},
		{"ids", op.IDs != nil},
		{"value", op.Value != ""},
		{"from", op.From != ""},
		{"to", op.To != ""},
	}
	var foreign []string
	for _, field := range populated {
		if field.set && !slices.Contains(allowed, field.name) {
			foreign = append(foreign, field.name)
		}
	}
	slices.Sort(foreign)
	return foreign
}

func findStepByID(items []PlanItem, id string) int {
	return slices.IndexFunc(items, func(item PlanItem) bool { return item.ID == id })
}

func sameStepOrder(old, new []PlanItem) bool {
	for i := range old {
		if old[i].ID != new[i].ID {
			return false
		}
	}
	return true
}
