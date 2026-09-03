package session

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// Action events: the lifecycle moments a built-in action can fire on. Step
// actions ride step transitions; plan actions ride approval and close.
const (
	PlanActionOnStepStart PlanActionEvent = "step_start"
	PlanActionOnStepEnd   PlanActionEvent = "step_end"
	PlanActionOnPlanStart PlanActionEvent = "plan_start"
	PlanActionOnPlanEnd   PlanActionEvent = "plan_end"
)

// Action types: the whole built-in command vocabulary. Deliberately small
// and environment-free — arbitrary shell commands are out of scope, so no
// action can spend a permission the user has not seen in the plan.
const (
	PlanActionCompact     PlanActionType = "compact"
	PlanActionInjectSkill PlanActionType = "inject_skill"
)

// Run statuses: the bounded terminal outcomes of one executed action.
const (
	PlanActionRunOK     PlanActionRunStatus = "ok"
	PlanActionRunFailed PlanActionRunStatus = "failed"
)

const (
	// Action lists are per-event hooks, not scripts: a short list per scope
	// keeps the sidebar chip line and the serialized budget honest. Plan and
	// step scopes share the cap today; split it only when a real need shows.
	maxPlanActions = 4

	maxPlanActionSkills     = 4
	maxPlanActionSkillRunes = 64
	// A plan automation renders as one transcript row everywhere: the same
	// local tool name in the session projector and the applied tool run.
	planActionToolName = "⚙ plan"

	// Model references are opaque to the session layer: the environment
	// (config models plus the provider catalog) owns name validation at the
	// tool seam; this layer owns the shape, so a name can never corrupt the
	// snapshot.
	maxPlanModelRunes = 128

	// Run history is a bounded tail like attempts: the compact index the
	// sidebar shows, not the audit — the session log is the audit.
	maxPlanActionRunsKept      = 8
	maxPlanActionRunErrorRunes = 512
)

// PlanActionEvent is the lifecycle moment one action fires on.
type PlanActionEvent string

// PlanActionType is one built-in command from the vocabulary above.
type PlanActionType string

// PlanActionRunStatus is the terminal outcome of one executed action.
type PlanActionRunStatus string

// PlanAction binds one built-in command to a lifecycle event. Skills is the
// inject_skill parameter: the named skills must be loaded before the event's
// turn proceeds. DisabledSkills is the user's off switch per name — the
// action keeps listing the skill so a toggle can come back, while injection
// and rendering read EffectiveSkills. Runs are harness-recorded execution
// history — authoring paths strip them, only AppendPlanActionRun writes them.
type PlanAction struct {
	Event          PlanActionEvent `json:"event"                    yaml:"event"`
	Type           PlanActionType  `json:"type"                     yaml:"type"`
	Skills         []string        `json:"skills,omitempty"         yaml:"skills,omitempty"`
	DisabledSkills []string        `json:"disabledSkills,omitempty" yaml:"disabledSkills,omitempty"`
	Runs           []PlanActionRun `json:"runs,omitempty"           yaml:"runs,omitempty"`
}

// PlanActionRun records one execution: the outcome, the actionable error
// when it failed, and when. Appended, never overwritten; the tail is
// bounded.
type PlanActionRun struct {
	Status PlanActionRunStatus `json:"status"`
	Error  string              `json:"error,omitempty"`
	At     time.Time           `json:"at"`
}

// planActionScope is which lifecycle an action list belongs to; the legal
// events depend on it. Keeping the events level-specific means a step action
// can never silently fire on approval.
type planActionScope string

const (
	planActionsStep planActionScope = "step"
	planActionsPlan planActionScope = "plan"
)

// events lists the scope's events for actionable error messages.
func (s planActionScope) events() string {
	if s == planActionsPlan {
		return "plan_start, plan_end"
	}
	return "step_start, step_end"
}

func (s planActionScope) allows(event PlanActionEvent) bool {
	step := event == PlanActionOnStepStart || event == PlanActionOnStepEnd
	plan := event == PlanActionOnPlanStart || event == PlanActionOnPlanEnd
	if s == planActionsPlan {
		return plan
	}
	return step
}

// knownPlanActionEvent reports whether the event exists at any level, so an
// event on the wrong scope can name its real home instead of crying unknown.
func knownPlanActionEvent(event PlanActionEvent) bool {
	switch event {
	case PlanActionOnStepStart, PlanActionOnStepEnd, PlanActionOnPlanStart, PlanActionOnPlanEnd:
		return true
	}
	return false
}

// normalizePlanActions validates one action list for its scope. Authoring
// paths pass keepRuns=false and lose any seeded run history; the load path
// keeps runs and validates them in place.
func normalizePlanActions(
	actions []PlanAction,
	scope planActionScope,
	where string,
	keepRuns bool,
) ([]PlanAction, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	if len(actions) > maxPlanActions {
		return nil, fmt.Errorf("session: %s has %d actions; maximum is %d", where, len(actions), maxPlanActions)
	}
	out := make([]PlanAction, len(actions))
	for i, action := range actions {
		what := fmt.Sprintf("%s action %d", where, i+1)
		if err := normalizePlanAction(&action, scope, what); err != nil {
			return nil, err
		}
		if keepRuns {
			if err := validatePlanActionRuns(action.Runs, what); err != nil {
				return nil, err
			}
		} else {
			action.Runs = nil
		}
		out[i] = action
	}
	return out, nil
}

// normalizePlanAction trims and validates one action definition.
func normalizePlanAction(action *PlanAction, scope planActionScope, what string) error {
	action.Event = PlanActionEvent(strings.TrimSpace(string(action.Event)))
	action.Type = PlanActionType(strings.TrimSpace(string(action.Type)))
	switch {
	case action.Event == "":
		return fmt.Errorf("session: %s event is required (use %s)", what, scope.events())
	case !scope.allows(action.Event):
		if knownPlanActionEvent(action.Event) {
			return fmt.Errorf("session: %s event %q belongs to the other plan level (use %s)",
				what, action.Event, scope.events())
		}
		return fmt.Errorf("session: %s has unknown event %q (use %s)", what, action.Event, scope.events())
	}
	switch action.Type {
	case PlanActionCompact:
		if len(action.Skills) > 0 {
			return fmt.Errorf("session: %s compact takes no skills", what)
		}
		if len(action.DisabledSkills) > 0 {
			return fmt.Errorf("session: %s compact takes no disabled skills", what)
		}
	case PlanActionInjectSkill:
		if len(action.Skills) == 0 {
			return fmt.Errorf("session: %s inject_skill requires %d..%d skills", what, 1, maxPlanActionSkills)
		}
		if len(action.Skills) > maxPlanActionSkills {
			return fmt.Errorf("session: %s has %d skills; maximum is %d", what, len(action.Skills), maxPlanActionSkills)
		}
		seen := make(map[string]struct{}, len(action.Skills))
		for j, skill := range action.Skills {
			trimmed, err := sanitizePlanProse(fmt.Sprintf("%s skill %d", what, j+1), strings.TrimSpace(skill))
			if err != nil {
				return err
			}
			if trimmed == "" {
				return fmt.Errorf("session: %s skill %d is required", what, j+1)
			}
			if utf8.RuneCountInString(trimmed) > maxPlanActionSkillRunes {
				return fmt.Errorf("session: %s skill %d exceeds %d characters", what, j+1, maxPlanActionSkillRunes)
			}
			if _, dup := seen[trimmed]; dup {
				return fmt.Errorf("session: %s lists skill %q twice", what, trimmed)
			}
			seen[trimmed] = struct{}{}
			action.Skills[j] = trimmed
		}
		action.DisabledSkills = normalizeDisabledSkills(action.DisabledSkills, seen)
	default:
		return fmt.Errorf("session: %s has unknown type %q (use %s, %s)",
			what, action.Type, PlanActionCompact, PlanActionInjectSkill)
	}
	return nil
}

// validatePlanActionRuns bounds the run tail on load. A file carrying more
// runs than this harness writes, or a status it never records, is not one of
// ours; loading it fails closed instead of trusting it.
func validatePlanActionRuns(runs []PlanActionRun, what string) error {
	if len(runs) > maxPlanActionRunsKept {
		return fmt.Errorf("session: %s has %d runs; maximum is %d", what, len(runs), maxPlanActionRunsKept)
	}
	for i, run := range runs {
		if !validPlanActionRunStatus(run.Status) {
			return fmt.Errorf("session: %s run %d has unknown status %q", what, i+1, run.Status)
		}
		if utf8.RuneCountInString(run.Error) > maxPlanActionRunErrorRunes {
			return fmt.Errorf("session: %s run %d error exceeds %d characters", what, i+1, maxPlanActionRunErrorRunes)
		}
	}
	return nil
}

func validPlanActionRunStatus(status PlanActionRunStatus) bool {
	return status == PlanActionRunOK || status == PlanActionRunFailed
}

// normalizeModelRef validates the shape of one model reference: unset is
// legal (the step follows the type map), blank is malformed authoring, and
// the name is bounded and masked. Existence is the tool layer's call.
func normalizeModelRef(field, value, where string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if value == "" {
			return "", nil
		}
		return "", fmt.Errorf("session: %s %s is blank", where, field)
	}
	if utf8.RuneCountInString(trimmed) > maxPlanModelRunes {
		return "", fmt.Errorf("session: %s %s exceeds %d characters", where, field, maxPlanModelRunes)
	}
	sanitized, err := sanitizePlanProse(field, trimmed)
	if err != nil {
		return "", err
	}
	return sanitized, nil
}

// validStepType reports whether the type is one of the plan's step types.
func validStepType(stepType StepType) bool {
	switch stepType {
	case StepExplore, StepEdit, StepRun, StepDelegate, StepIntegrate:
		return true
	}
	return false
}

// normalizeModelsByType validates the per-step-type model map: known types
// only, every entry a real (non-blank, bounded) model reference. An empty
// map normalizes to nil so absence has one representation.
func normalizeModelsByType(models map[StepType]string) (map[StepType]string, error) {
	if len(models) == 0 {
		return nil, nil
	}
	out := make(map[StepType]string, len(models))
	for stepType, model := range models {
		if !validStepType(stepType) {
			return nil, fmt.Errorf("session: plan modelsByType has unknown step type %q", stepType)
		}
		ref, err := normalizeModelRef("model", model, fmt.Sprintf("modelsByType %s", stepType))
		if err != nil {
			return nil, err
		}
		if ref == "" {
			return nil, fmt.Errorf("session: plan modelsByType %s model is required", stepType)
		}
		out[stepType] = ref
	}
	return out, nil
}

// AppendPlanActionRun durably records one executed action run. The empty
// step id addresses the plan-level list. Runs are operational evidence like
// attempts: appended, tail-bounded, never moving approval or lifecycle.
func (sm *Manager) AppendPlanActionRun(stepID string, actionIndex int, run PlanActionRun) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	stepID = strings.TrimSpace(stepID)
	if err := normalizePlanActionRun(&run); err != nil {
		return Plan{}, fmt.Errorf("session: record action run: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.plan.Schema.IsV2() {
		return Plan{}, errors.New(
			"session: plan actions require a v2 plan; send the full contract with action create",
		)
	}

	plan := sm.plan.Clone()
	var actions *[]PlanAction
	if stepID == "" {
		actions = &plan.Actions
	} else {
		idx := findStepByID(plan.Items, stepID)
		if idx < 0 {
			return Plan{}, fmt.Errorf("session: step %q not found", stepID)
		}
		actions = &plan.Items[idx].Actions
	}
	if actionIndex < 0 || actionIndex >= len(*actions) {
		return Plan{}, fmt.Errorf("session: plan action %d not found", actionIndex+1)
	}
	(*actions)[actionIndex].Runs = appendPlanActionRunRecord((*actions)[actionIndex].Runs, run)
	plan.Revision = sm.plan.Revision + 1
	plan.UpdatedAt = run.At
	// The run rides inside the snapshot, so the record owns the serialized
	// budget the same way authoring does.
	if err := planWithinSerializedBudget(plan); err != nil {
		return Plan{}, err
	}
	return sm.persistPlanLocked(plan)
}

// SetPlanSkillDisabled toggles one skill's off mark on a step's inject_skill
// action in place: Skills, run history, and the rest of the list stay exactly
// as they are, so a user toggle never retires recorded runs. Toggling is
// material — commitPlanLocked decides approval from the diff, and an off mark
// counts. The empty step id addresses the plan-level list.
func (sm *Manager) SetPlanSkillDisabled(stepID string, actionIndex int, skill string, disabled bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	stepID = strings.TrimSpace(stepID)
	skill = strings.TrimSpace(skill)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.plan.Schema.IsV2() {
		return Plan{}, errors.New(
			"session: plan actions require a v2 plan; send the full contract with action create",
		)
	}

	plan := sm.plan.Clone()
	var actions *[]PlanAction
	if stepID == "" {
		actions = &plan.Actions
	} else {
		idx := findStepByID(plan.Items, stepID)
		if idx < 0 {
			return Plan{}, fmt.Errorf("session: step %q not found", stepID)
		}
		actions = &plan.Items[idx].Actions
	}
	if actionIndex < 0 || actionIndex >= len(*actions) {
		return Plan{}, fmt.Errorf("session: plan action %d not found", actionIndex+1)
	}
	action := &(*actions)[actionIndex]
	if action.Type != PlanActionInjectSkill {
		return Plan{}, fmt.Errorf(
			"session: plan action %d is %q; only inject_skill carries skills", actionIndex+1, action.Type,
		)
	}
	if !slices.Contains(action.Skills, skill) {
		return Plan{}, fmt.Errorf("session: plan action %d does not list skill %q", actionIndex+1, skill)
	}
	if disabled {
		if !slices.Contains(action.DisabledSkills, skill) {
			action.DisabledSkills = append(action.DisabledSkills, skill)
		}
	} else {
		action.DisabledSkills = slices.DeleteFunc(action.DisabledSkills, func(name string) bool {
			return name == skill
		})
	}
	plan.UpdatedAt = time.Now()
	committed, _, err := sm.commitPlanLocked(plan, false)
	if err != nil {
		return Plan{}, err
	}
	return committed, nil
}

// HasPlanMutation reports whether the mutation ledger already recorded this
// id. The engine consults it before running action automation: a replayed
// write carries no new state, so its side effects must not run again.
func (sm *Manager) HasPlanMutation(mutationID string) bool {
	if sm == nil || mutationID == "" {
		return false
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, found := findPlanMutation(sm.plan.Mutations, mutationID)
	return found
}

// normalizePlanActionRun validates one incoming record. The error text is
// harness-authored, so an over-long one is truncated, not rejected — same
// policy as attempt summaries.
func normalizePlanActionRun(run *PlanActionRun) error {
	if !validPlanActionRunStatus(run.Status) {
		return fmt.Errorf("unknown run status %q", run.Status)
	}
	if run.Error != "" {
		run.Error = boundRunError(stripControlChars(redact.Redact(strings.TrimSpace(run.Error))))
	}
	if run.At.IsZero() {
		run.At = time.Now()
	}
	return nil
}

// boundRunError truncates the error to its budget, marking the cut.
func boundRunError(msg string) string {
	if utf8.RuneCountInString(msg) <= maxPlanActionRunErrorRunes {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:maxPlanActionRunErrorRunes-3]) + "..."
}

// appendPlanActionRunRecord appends to the bounded tail; the oldest records
// drop off past the bound.
func appendPlanActionRunRecord(runs []PlanActionRun, run PlanActionRun) []PlanActionRun {
	runs = append(runs, run)
	if excess := len(runs) - maxPlanActionRunsKept; excess > 0 {
		runs = runs[excess:]
	}
	return runs
}

// ClonePlanActions returns a snapshot whose action lists do not alias the
// source. Exported for the plan-gate policy, which freezes default actions
// into an immutable snapshot.
func ClonePlanActions(actions []PlanAction) []PlanAction {
	out := slices.Clone(actions)
	for i := range out {
		out[i].Skills = slices.Clone(out[i].Skills)
		out[i].DisabledSkills = slices.Clone(out[i].DisabledSkills)
		out[i].Runs = slices.Clone(out[i].Runs)
	}
	return out
}

// NormalizePlanDefaultActions validates a plan-level action list authored as
// plan defaults (events plan_start / plan_end) and returns a detached copy
// with no run history: defaults define automation, they never record it.
func NormalizePlanDefaultActions(actions []PlanAction) ([]PlanAction, error) {
	return normalizeDefaultActions(actions, planActionsPlan, "plan defaults")
}

// NormalizeStepDefaultActions validates a step-level action list authored as
// plan defaults (events step_start / step_end); same detachment rules.
func NormalizeStepDefaultActions(actions []PlanAction) ([]PlanAction, error) {
	return normalizeDefaultActions(actions, planActionsStep, "step defaults")
}

func normalizeDefaultActions(actions []PlanAction, scope planActionScope, where string) ([]PlanAction, error) {
	normalized, err := normalizePlanActions(actions, scope, where, false)
	if err != nil {
		return nil, err
	}
	// The normalized list can still alias the input's skill slices (trimming
	// happens in place), so clone before the caller freezes the result.
	return ClonePlanActions(normalized), nil
}

// compileStepSkills folds an authored step skills list into the step's
// inject_skill@step_start action and clears the input field: Actions is the
// one canonical home for skills. Presence is authorship — the list displaces
// whatever the type defaults seeded, and the empty list removes the injection
// outright — while absence leaves the seeded automation standing. Off marks
// survive only for names the new list still carries; the orphan sweep in
// normalizePlanActions retires the rest.
func compileStepSkills(item *PlanItem) {
	if item.Skills == nil {
		return
	}
	authored := item.Skills
	item.Skills = nil
	replaced := false
	kept := make([]PlanAction, 0, len(item.Actions)+1)
	for _, action := range item.Actions {
		if action.Event != PlanActionOnStepStart || action.Type != PlanActionInjectSkill {
			kept = append(kept, action)
			continue
		}
		if len(authored) == 0 {
			continue // the author's explicit "none": the injection goes
		}
		action.Skills = authored
		kept = append(kept, action)
		replaced = true
	}
	if len(authored) > 0 && !replaced {
		kept = append(kept, PlanAction{
			Event: PlanActionOnStepStart, Type: PlanActionInjectSkill, Skills: authored,
		})
	}
	item.Actions = kept
}

// normalizeDisabledSkills trims the off list and keeps only names the action
// still lists, once each: a name dropped from Skills orphans its off mark, and
// re-authoring must not resurrect it. Authoring stays forgiving — orphans and
// blanks drop silently rather than failing the whole plan.
func normalizeDisabledSkills(disabled []string, listed map[string]struct{}) []string {
	var out []string
	seenDisabled := make(map[string]struct{}, len(disabled))
	for _, name := range disabled {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, stillListed := listed[trimmed]; !stillListed {
			continue
		}
		if _, dup := seenDisabled[trimmed]; dup {
			continue
		}
		seenDisabled[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// EffectiveSkills lists the skills this action still injects: Skills in their
// authored order minus the user's off marks. Injection and rendering read
// this; authoring paths keep the full list so a toggle can come back.
func (a PlanAction) EffectiveSkills() []string {
	if len(a.DisabledSkills) == 0 {
		return a.Skills
	}
	off := make(map[string]struct{}, len(a.DisabledSkills))
	for _, name := range a.DisabledSkills {
		off[name] = struct{}{}
	}
	effective := make([]string, 0, len(a.Skills))
	for _, name := range a.Skills {
		if _, disabled := off[name]; !disabled {
			effective = append(effective, name)
		}
	}
	return effective
}

// PlanActionEqual compares two action definitions, ignoring run history and
// the disabled-skill set: it answers "is this the same automation whose runs
// were recorded", which is what run restoration and change detection need.
// The material diff compares with sameActionDefinition, which also sees the
// disabled set.
func PlanActionEqual(a, b PlanAction) bool {
	return a.Event == b.Event && a.Type == b.Type && slices.Equal(a.Skills, b.Skills)
}

// restorePlanActionRuns puts harness-recorded run history back onto a
// normalized plan, but only onto actions whose definitions survived: a
// re-authored action list is a new contract and starts with an empty
// history.
func restorePlanActionRuns(dst *[]PlanAction, original []PlanAction) {
	for i := range original {
		if i >= len(*dst) || len(original[i].Runs) == 0 {
			continue
		}
		if !PlanActionEqual((*dst)[i], original[i]) {
			continue
		}
		(*dst)[i].Runs = slices.Clone(original[i].Runs)
	}
}
