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
// turn proceeds. Runs are harness-recorded execution history — authoring
// paths strip them, only AppendPlanActionRun writes them.
type PlanAction struct {
	Event  PlanActionEvent `json:"event"`
	Type   PlanActionType  `json:"type"`
	Skills []string        `json:"skills,omitempty"`
	Runs   []PlanActionRun `json:"runs,omitempty"`
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

// clonePlanActions returns a snapshot whose action lists do not alias the
// source.
func clonePlanActions(actions []PlanAction) []PlanAction {
	out := slices.Clone(actions)
	for i := range out {
		out[i].Skills = slices.Clone(out[i].Skills)
		out[i].Runs = slices.Clone(out[i].Runs)
	}
	return out
}

// PlanActionEqual compares two action definitions, ignoring run history: it
// answers "is this the same automation the user approved", which is what the
// material diff, run restoration and the plan editor's change detection all
// need.
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
