// Package plantool exposes the durable session plan to the primary model.
package plantool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Deps binds the model tool to the engine's current session.
type Deps struct {
	Update     func(context.Context, []session.PlanItem) (session.Plan, error)
	Create     func(context.Context, session.PlanV2) (session.Plan, []session.PlanMaterialChange, error)
	Get        func(context.Context) (session.Plan, error)
	Patch      func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error)
	Transition func(context.Context, session.PlanTransition) (session.Plan, session.PlanTransitionResult, error)
	// Telemetry sources the bounded observability snapshot for view=telemetry;
	// nil = a zero snapshot, never an error — observability degrades first.
	Telemetry func(context.Context) (plantel.Snapshot, error)
	StepTypes []string
	// Models lists the model names a pin may reference; nil skips the
	// authoring check and leaves step-start resolution as the backstop.
	Models func() []string
	// Skills lists the skill names inject_skill actions may reference; nil
	// skips the same way.
	Skills func() []string
}

// snapshot is the legacy update answer: the canonical items plus a marker that
// the call ran on the steps-only compatibility path.
type snapshot struct {
	Revision      uint64             `json:"revision"`
	Approved      bool               `json:"approved"`
	Items         []session.PlanItem `json:"items"`
	Compatibility string             `json:"compatibility,omitempty"`
}

// input decodes the discriminated plan contract. One struct carries all three
// actions so strict decoding still rejects unknown fields once, at the seam.
// The legacy expected_revision stays recognized so a resumed session that
// replays an old update call does not trip; it carries no authority and is
// dropped on read.
type input struct {
	Action           string                      `json:"action"`
	View             string                      `json:"view"`
	ExpectedRevision *uint64                     `json:"expected_revision"`
	Steps            []session.PlanItem          `json:"steps"`
	Goal             string                      `json:"goal"`
	Approach         string                      `json:"approach"`
	SuccessCriteria  []string                    `json:"successCriteria"`
	Constraints      []string                    `json:"constraints"`
	WorkingContext   string                      `json:"workingContext"`
	Actions          []session.PlanAction        `json:"actions"`
	ModelsByType     map[session.StepType]string `json:"modelsByType"`
	Ops              []session.PlanPatchOp       `json:"ops"`

	// Lifecycle payload: start, complete, block, resume, cancel, reopen.
	ID               string   `json:"id"`
	MutationID       string   `json:"mutationId"`
	Outcome          string   `json:"outcome"`
	Evidence         string   `json:"evidence"`
	EvidenceRefs     []string `json:"evidenceRefs"`
	NoEvidenceReason string   `json:"noEvidenceReason"`
	Blocker          string   `json:"blocker"`
	ResumeWhen       string   `json:"resumeWhen"`
	Reason           string   `json:"reason"`
	PlanResult       string   `json:"planResult"`
}

// hasTransitionFields reports whether any lifecycle payload rode along. The
// fields belong to the transition actions only; every other action refuses
// them instead of silently dropping work the model believes it sent.
func (in input) hasTransitionFields() bool {
	return in.ID != "" || in.MutationID != "" || in.Outcome != "" || in.Evidence != "" ||
		in.EvidenceRefs != nil || in.NoEvidenceReason != "" || in.Blocker != "" ||
		in.ResumeWhen != "" || in.Reason != "" || in.PlanResult != ""
}

// hasNonDefaultTransitionFields ignores zero values materialized by a model
// client for create/update while still refusing meaningful lifecycle input.
func (in input) hasNonDefaultTransitionFields() bool {
	return in.ID != "" || in.MutationID != "" || in.Outcome != "" || in.Evidence != "" ||
		len(in.EvidenceRefs) > 0 || in.NoEvidenceReason != "" || in.Blocker != "" ||
		in.ResumeWhen != "" || in.Reason != "" || in.PlanResult != ""
}

// isTransitionAction reports whether the action is one of the six lifecycle
// moves the session state machine owns.
func isTransitionAction(action string) bool {
	switch action {
	case session.TransitionStart, session.TransitionComplete, session.TransitionBlock,
		session.TransitionResume, session.TransitionCancel, session.TransitionReopen:
		return true
	}
	return false
}

// hasContractFields reports whether any v2 contract field rode along. The
// legacy update path must refuse them instead of silently dropping work the
// model believes it sent.
func (in input) hasContractFields() bool {
	return in.Goal != "" || in.Approach != "" || in.WorkingContext != "" ||
		in.SuccessCriteria != nil || in.Constraints != nil ||
		in.Actions != nil || in.ModelsByType != nil
}

// hasNonDefaultContractFields is the update-path variant: empty arrays are
// provider defaults, not an attempt to mutate the v2 contract.
func (in input) hasNonDefaultContractFields() bool {
	return in.Goal != "" || in.Approach != "" || in.WorkingContext != "" ||
		len(in.SuccessCriteria) > 0 || len(in.Constraints) > 0 ||
		len(in.Actions) > 0 || len(in.ModelsByType) > 0
}

// stepsCarryV2Fields reports whether any step rides v2 contract metadata or
// harness-recorded evidence. The legacy path strips these fields durably, so
// their presence must be refused here rather than silently dropped after the
// model believed it sent them.
func stepsCarryV2Fields(items []session.PlanItem) bool {
	for _, item := range items {
		if item.ID != "" || item.Why != "" || item.DoneWhen != "" ||
			item.Risk != "" || item.Outcome != "" || item.JIT || item.EvidenceRefs != nil ||
			item.Blocker != "" || item.ResumeWhen != "" || item.Attempts != nil ||
			item.Model != "" || item.Actions != nil {
			return true
		}
	}
	return false
}

// actionsCarryRuns reports whether authored actions smuggle run history.
// Runs are harness-recorded evidence; the model cannot forge them.
func actionsCarryRuns(actions []session.PlanAction) bool {
	for _, action := range actions {
		if action.Runs != nil {
			return true
		}
	}
	return false
}

// validateModelPin refuses a model name the environment cannot resolve: the
// tool seam is where the error can list the valid options, long before a
// step start refuses the same name.
func validateModelPin(known func() []string, where, name string) error {
	if name == "" || known == nil {
		return nil
	}
	names := known()
	if slices.Contains(names, name) {
		return nil
	}
	return fmt.Errorf("plan: %s pins model %q, which is not configured; configured models: %s",
		where, name, strings.Join(names, ", "))
}

// validateSkills refuses skill names the environment does not carry; a typo
// in an inject_skill pin surfaces here, not as a silently skipped read.
func validateSkills(known func() []string, where string, skills []string) error {
	if known == nil {
		return nil
	}
	available := known()
	for _, skill := range skills {
		if !slices.Contains(available, skill) {
			return fmt.Errorf("plan: %s references skill %q, which is not installed; available skills: %s",
				where, skill, strings.Join(available, ", "))
		}
	}
	return nil
}

func validateActionPins(deps Deps, where string, actions []session.PlanAction) error {
	for _, action := range actions {
		if action.Type != session.PlanActionInjectSkill {
			continue
		}
		if err := validateSkills(deps.Skills, where, action.Skills); err != nil {
			return err
		}
	}
	return nil
}

// validateContractPins fails closed on model and skill names the environment
// cannot resolve across a whole-contract write.
func (deps Deps) validateContractPins(contract session.PlanV2) error {
	for typ, name := range contract.ModelsByType {
		if err := validateModelPin(deps.Models, fmt.Sprintf("modelsByType[%s]", typ), name); err != nil {
			return err
		}
	}
	if err := validateActionPins(deps, "plan actions", contract.Actions); err != nil {
		return err
	}
	for _, item := range contract.Items {
		if err := validateModelPin(deps.Models, fmt.Sprintf("step %q", item.ID), item.Model); err != nil {
			return err
		}
		if err := validateActionPins(deps, fmt.Sprintf("step %q actions", item.ID), item.Actions); err != nil {
			return err
		}
	}
	return nil
}

// validatePatchPins does the same for a patch batch: update_step model and
// action lists, set_plan_fields modelsByType, and inserted steps.
func (deps Deps) validatePatchPins(ops []session.PlanPatchOp) error {
	for _, op := range ops {
		if op.Model.Set {
			if err := validateModelPin(deps.Models, fmt.Sprintf("step %q", op.ID), op.Model.Value); err != nil {
				return err
			}
		}
		if op.ModelsByType.Set {
			for typ, name := range op.ModelsByType.Value {
				if err := validateModelPin(deps.Models, fmt.Sprintf("modelsByType[%s]", typ), name); err != nil {
					return err
				}
			}
		}
		if op.Actions.Set {
			if err := validateActionPins(deps, fmt.Sprintf("step %q actions", op.ID), op.Actions.Value); err != nil {
				return err
			}
		}
		if op.Step != nil {
			if err := validateModelPin(deps.Models, fmt.Sprintf("step %q", op.Step.ID), op.Step.Model); err != nil {
				return err
			}
			if err := validateActionPins(
				deps,
				fmt.Sprintf("step %q actions", op.Step.ID),
				op.Step.Actions,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasNonDefaultView reports a real response-shape override. Some providers
// materialize the advertised "active" default for every action; it remains
// semantically absent outside get, while "full" must still be rejected.
func hasNonDefaultView(view string) bool {
	return view != "" && view != "active"
}

// actionSchema describes one actions slot of the model-facing definition. The
// bounds mirror the session layer's unexported budget
// (internal/session/plan_action.go); the golden definition test pins them.
func actionSchema(events []string, level string) llm.Object {
	return llm.Object{
		"type":     "array",
		"maxItems": 4,
		"description": fmt.Sprintf(
			"Built-in automations bound to %s; the harness runs them at the event, and a failed action rejects the transition.",
			level,
		),
		"items": llm.Object{
			"type": "object",
			"properties": llm.Object{
				"event": llm.Object{
					"type":        "string",
					"enum":        events,
					"description": "Lifecycle moment the action fires on.",
				},
				"type": llm.Object{
					"type":        "string",
					"enum":        actionTypes,
					"description": "compact runs context compaction; inject_skill loads named skills before the step's first turn.",
				},
				"skills": llm.Object{
					"type":        "array",
					"maxItems":    4,
					"description": "inject_skill: 1-4 skill names; compact carries none.",
					"items":       llm.Object{"type": "string", "maxLength": 64},
				},
			},
			"required": []string{"event", "type"},
		},
	}
}

// modelSchema describes one model pin: a step override or a per-type entry.
func modelSchema(description string) llm.Object {
	return llm.Object{"type": "string", "maxLength": 128, "description": description}
}

// Event and action-type enums derive from the session layer so the definition
// cannot drift from the validation that backs it.
var (
	planActionEvents  = []string{string(session.PlanActionOnPlanStart), string(session.PlanActionOnPlanEnd)}
	stepActionEvents  = []string{string(session.PlanActionOnStepStart), string(session.PlanActionOnStepEnd)}
	patchActionEvents = append(append([]string{}, stepActionEvents...), planActionEvents...)
	actionTypes       = []string{string(session.PlanActionCompact), string(session.PlanActionInjectSkill)}
)

// Tool returns the model-facing interface to the canonical durable plan:
// create sends a full v2 work contract as an unapproved draft, get reads a
// bounded view of the current plan, patch atomically applies domain-specific
// operations against an expected revision, the lifecycle actions move one
// step through the validated state machine, and update keeps the legacy
// steps-only replacement. The harness owns the revision; in a v2 plan, after
// create, status moves only through the lifecycle actions.
func Tool(deps Deps) tooldef.Tool {
	unavailable := errors.New("session plan unavailable")
	if deps.Update == nil {
		deps.Update = func(context.Context, []session.PlanItem) (session.Plan, error) {
			return session.Plan{}, unavailable
		}
	}
	if deps.Create == nil {
		deps.Create = func(context.Context, session.PlanV2) (session.Plan, []session.PlanMaterialChange, error) {
			return session.Plan{}, nil, unavailable
		}
	}
	if deps.Get == nil {
		deps.Get = func(context.Context) (session.Plan, error) {
			return session.Plan{}, unavailable
		}
	}
	if deps.Patch == nil {
		deps.Patch = func(context.Context, uint64, []session.PlanPatchOp) (session.Plan, session.PlanPatchSummary, error) {
			return session.Plan{}, session.PlanPatchSummary{}, unavailable
		}
	}
	if deps.Transition == nil {
		deps.Transition = func(context.Context, session.PlanTransition) (session.Plan, session.PlanTransitionResult, error) {
			return session.Plan{}, session.PlanTransitionResult{}, unavailable
		}
	}
	if deps.Telemetry == nil {
		deps.Telemetry = func(context.Context) (plantel.Snapshot, error) {
			return plantel.Snapshot{}, nil
		}
	}
	stepTypes := deps.StepTypes
	if stepTypes == nil {
		// Standalone callers (tests) get the built-in policy instead of a stale
		// literal copy of it; engines always pass the live runtime types.
		stepTypes = plangate.DefaultDefaults().StepTypeNames()
	}
	typeModels := llm.Object{}
	for _, stepType := range stepTypes {
		typeModels[stepType] = modelSchema("Model for this step type.")
	}
	modelsByTypeSchema := llm.Object{
		"type":        "object",
		"description": "Model per step type; a step's model override wins, unlisted types follow the session model.",
		"properties":  typeModels,
	}

	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan",
			Description: "Create, read, patch, transition, or replace the durable plan. action=create sends the full work contract (goal, approach, successCriteria, steps with id/why/doneWhen) and starts an unapproved draft; action=get returns the compact projection (view=full returns the canonical snapshot with audit history); action=patch atomically applies ops addressed by stable step ids against expected_revision and answers with the changed delta; the lifecycle actions start/complete/block/resume/cancel/reopen move one step by id (complete carries outcome plus evidence or no_evidence_reason, and a call:<callId> evidence ref must cite a recorded successful attempt, block carries blocker and resume_when, cancel and reopen carry reason) and replay recorded results for a repeated mutationId; complete with planResult also closes the finished plan in the same write (the bounded terminal view then replaces the projection), and reopen without id restores a closed plan; action=update replaces the ordered steps only (legacy steps-only shape). Steps and the plan can pin built-in actions (compact, inject_skill) to lifecycle events; the harness runs them before the transition's durable write, and a failure rejects the move. A step model override or a modelsByType entry pins the model a step runs on. The harness owns the revision; in a v2 plan, after create, status moves only through the lifecycle actions. Plan prose holds concise operational rationale — never secrets, raw logs, or raw chain-of-thought; cite bounded evidence refs instead.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type":        "string",
						"description": "Discriminates the call: create sends the full work contract, get reads the current plan, patch applies atomic ops against expected_revision, start/complete/block/resume/cancel/reopen move one step through the lifecycle, update replaces the ordered steps only (legacy).",
						"enum": []string{
							"create",
							"get",
							"patch",
							"update",
							"start",
							"complete",
							"block",
							"resume",
							"cancel",
							"reopen",
						},
					},
					"expected_revision": llm.Object{
						"type":        "integer",
						"description": "Revision the patch expects; required for action patch. A stale value returns the actual revision.",
					},
					"view": llm.Object{
						"type":        "string",
						"description": "Response shape for action get; default active. The telemetry view is the bounded observability snapshot: counters only, no plan content.",
						"enum":        []string{"active", "full", "telemetry"},
					},
					"goal": llm.Object{
						"type":        "string",
						"description": "One-sentence outcome the plan exists to reach; required for create.",
						"maxLength":   512,
					},
					"approach": llm.Object{
						"type":        "string",
						"description": "Chosen strategy in brief; required for create.",
						"maxLength":   1024,
					},
					"successCriteria": llm.Object{
						"type":        "array",
						"description": "Observable conditions that prove the goal; at least one; required for create.",
						"maxItems":    8,
						"items":       llm.Object{"type": "string", "maxLength": 512},
					},
					"constraints": llm.Object{
						"type":        "array",
						"description": "Hard limits the plan must respect.",
						"maxItems":    8,
						"items":       llm.Object{"type": "string", "maxLength": 512},
					},
					"workingContext": llm.Object{
						"type":        "string",
						"description": "Bounded context the steps assume.",
						"maxLength":   2048,
					},
					"actions": actionSchema(planActionEvents,
						"the whole plan (create); patch ops set them via set_plan_fields"),
					"modelsByType": modelsByTypeSchema,
					"steps": llm.Object{
						"type":        "array",
						"description": "Complete ordered plan snapshot; maximum 32 steps.",
						"maxItems":    32,
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"content": llm.Object{
									"type":        "string",
									"description": "Specific actionable step; maximum 512 characters.",
									"maxLength":   512,
								},
								"status": llm.Object{
									"type": "string",
									"enum": []string{
										"pending", "in_progress", "blocked", "completed", "cancelled",
									},
								},
								"type": llm.Object{
									"type":        "string",
									"description": "What this step is allowed to do.",
									"enum":        stepTypes,
								},
								"note": llm.Object{
									"type":        "string",
									"description": "Optional concise finding, assumption, or blocker reason; maximum 512 characters.",
									"maxLength":   512,
								},
								"evidence": llm.Object{
									"type":        "string",
									"description": "Optional concise proof or verification result; maximum 512 characters.",
									"maxLength":   512,
								},
								"id": llm.Object{
									"type":        "string",
									"description": "Stable slug identifying this step; required for create.",
									"maxLength":   64,
								},
								"why": llm.Object{
									"type":        "string",
									"description": "Why this step exists; required for create.",
									"maxLength":   512,
								},
								"doneWhen": llm.Object{
									"type":        "string",
									"description": "Observable condition that ends this step; required for create.",
									"maxLength":   512,
								},
								"risk": llm.Object{
									"type":        "string",
									"description": "What could go wrong and the blast radius.",
									"maxLength":   512,
								},
								"jit": llm.Object{
									"type":        "boolean",
									"description": "True when the step is irreversible and needs just-in-time approval.",
								},
								"model": modelSchema(
									"Model override for this step; empty follows modelsByType for the step's type, then the session model.",
								),
								"actions": actionSchema(stepActionEvents, "this step"),
							},
							"required": []string{"content", "status", "type"},
						},
					},
					"ops": llm.Object{
						// The bounds below mirror the session layer's unexported
						// budget (internal/session/plan.go); the golden definition
						// test pins them so drift is caught in review.
						"type":        "array",
						"description": "Atomic patch batch for action patch; maximum 32 ops, applied all-or-none against expected_revision. Each op reads only its own fields; scalar slots: absent keeps the value, a value replaces it, JSON null clears an optional one.",
						"maxItems":    32,
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"op": llm.Object{
									"type": "string",
									"enum": []string{
										"set_plan_fields", "replace_context", "update_step", "insert_step",
										"remove_step", "reorder_steps",
										"add_constraint", "update_constraint", "remove_constraint",
										"add_criterion", "update_criterion", "remove_criterion",
									},
								},
								"goal": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "set_plan_fields.",
								},
								"approach": llm.Object{
									"type":        "string",
									"maxLength":   1024,
									"description": "set_plan_fields.",
								},
								"workingContext": llm.Object{
									"type":        "string",
									"maxLength":   2048,
									"description": "replace_context: the whole working context; null or empty clears it.",
								},
								"modelsByType": modelsByTypeSchema,
								"actions": actionSchema(
									patchActionEvents,
									"update_step (step-level events) or set_plan_fields (plan-level events); replaces the list, null clears",
								),
								"model": modelSchema(
									"update_step: model override for the step; empty follows the type map, null clears.",
								),
								"id": llm.Object{
									"type":        "string",
									"description": "update_step / remove_step target step id.",
								},
								"content": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_step.",
								},
								"why": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_step.",
								},
								"doneWhen": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_step.",
								},
								"risk": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_step; optional, null clears.",
								},
								"note": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_step operational note; optional, null clears.",
								},
								"before": llm.Object{
									"type":        "string",
									"description": "insert_step anchor: place the new step before this id.",
								},
								"after": llm.Object{
									"type":        "string",
									"description": "insert_step anchor: place the new step after this id.",
								},
								"step": llm.Object{
									"type":        "object",
									"description": "insert_step payload; starts pending.",
									"properties": llm.Object{
										"id": llm.Object{
											"type":        "string",
											"maxLength":   64,
											"description": "Stable slug; required.",
										},
										"content": llm.Object{
											"type":        "string",
											"maxLength":   512,
											"description": "Required.",
										},
										"type": llm.Object{
											"type":        "string",
											"enum":        stepTypes,
											"description": "Required.",
										},
										"why": llm.Object{
											"type":        "string",
											"maxLength":   512,
											"description": "Required.",
										},
										"doneWhen": llm.Object{
											"type":        "string",
											"maxLength":   512,
											"description": "Required.",
										},
										"risk":    llm.Object{"type": "string", "maxLength": 512},
										"jit":     llm.Object{"type": "boolean"},
										"model":   modelSchema("Model override; empty follows the type map."),
										"actions": actionSchema(stepActionEvents, "this step"),
									},
									"required": []string{"id", "content", "type", "why", "doneWhen"},
								},
								"ids": llm.Object{
									"type":        "array",
									"maxItems":    32,
									"description": "reorder_steps: the complete new order of every step id.",
									"items":       llm.Object{"type": "string", "maxLength": 64},
								},
								"value": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "add_/remove_ directive text (its identity).",
								},
								"from": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_ directive current text.",
								},
								"to": llm.Object{
									"type":        "string",
									"maxLength":   512,
									"description": "update_ directive replacement text.",
								},
							},
							"required": []string{"op"},
						},
					},
					// The lifecycle bounds below mirror the session layer's
					// unexported budget (internal/session/plan.go); the golden
					// definition test pins them so drift is caught in review.
					"id": llm.Object{
						"type":        "string",
						"maxLength":   64,
						"description": "Lifecycle target step id; required for start/complete/block/resume/cancel/reopen. Reopen without id addresses the closed plan itself.",
					},
					"mutationId": llm.Object{
						"type":        "string",
						"maxLength":   64,
						"description": "Idempotency key for one lifecycle action; a retry with the same id replays the recorded result.",
					},
					"outcome": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "complete: concise result the step produced; required.",
					},
					"evidence": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "complete: concise proof; required unless evidence_refs or no_evidence_reason is sent.",
					},
					"evidenceRefs": llm.Object{
						"type":        "array",
						"maxItems":    8,
						"description": "complete: bounded artifacts that prove the outcome; cite a recorded successful attempt as call:<its callId>.",
						"items":       llm.Object{"type": "string", "maxLength": 128},
					},
					"noEvidenceReason": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "complete: why no evidence can exist; only valid without evidence.",
					},
					"blocker": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "block: what blocks the step; required.",
					},
					"resumeWhen": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "block: the condition that unblocks the step; required.",
					},
					"reason": llm.Object{
						"type":        "string",
						"maxLength":   512,
						"description": "cancel / reopen: why; required.",
					},
					"planResult": llm.Object{
						"type":        "string",
						"enum":        []string{"success", "abandoned"},
						"description": "complete: close the whole plan in the same write when this step is the last active work; success asserts the success criteria are met. Refused while any step is pending, in_progress or blocked, or (for success) when a step was cancelled.",
					},
				},
				Required: []string{"action"},
			},
		},
		DetailFromArgs: detailFromArgs,
		Run: func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
			var in input
			if err := tooldef.DecodeStrict(raw, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("plan args: %w", err)
			}
			transitionFields := in.hasTransitionFields()
			if in.Action == "create" || in.Action == "" || in.Action == "update" {
				transitionFields = in.hasNonDefaultTransitionFields()
			}
			if !isTransitionAction(in.Action) && transitionFields {
				return tooldef.Result{}, errors.New(
					"plan: transition fields need one of the lifecycle actions " +
						"(start, complete, block, resume, cancel, reopen)",
				)
			}
			switch in.Action {
			case "create":
				return runCreate(ctx, deps, in)
			case "get":
				return runGet(ctx, deps, in)
			case "patch":
				return runPatch(ctx, deps, in)
			case session.TransitionStart, session.TransitionComplete, session.TransitionBlock,
				session.TransitionResume, session.TransitionCancel, session.TransitionReopen:
				return runTransition(ctx, deps, in)
			case "", "update":
				return runUpdate(ctx, deps, in)
			default:
				return tooldef.Result{}, fmt.Errorf(
					"plan: unsupported action %q (use create, get, patch, update, "+
						"start, complete, block, resume, cancel, or reopen)", in.Action,
				)
			}
		},
	}
}

// runCreate maps the request onto the v2 contract and stores an unapproved
// draft. The session layer owns every required-field text; the tool wraps.
func runCreate(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if hasNonDefaultView(in.View) {
		return tooldef.Result{}, errors.New("plan create: view is only valid with action get")
	}
	if in.Steps == nil {
		return tooldef.Result{}, errors.New("plan create: steps is required")
	}
	for _, item := range in.Steps {
		if item.Attempts != nil {
			return tooldef.Result{}, errors.New(
				"plan create: steps take no attempts; the harness records them from accepted tool calls",
			)
		}
		if actionsCarryRuns(item.Actions) {
			return tooldef.Result{}, errors.New(
				"plan create: step actions take no runs; the harness records them from executed transitions",
			)
		}
	}
	if actionsCarryRuns(in.Actions) {
		return tooldef.Result{}, errors.New(
			"plan create: plan actions take no runs; the harness records them from executed transitions",
		)
	}
	contract := session.PlanV2{
		Goal:            in.Goal,
		Approach:        in.Approach,
		SuccessCriteria: in.SuccessCriteria,
		Constraints:     in.Constraints,
		WorkingContext:  in.WorkingContext,
		Actions:         in.Actions,
		ModelsByType:    in.ModelsByType,
		Items:           in.Steps,
	}
	if err := deps.validateContractPins(contract); err != nil {
		return tooldef.Result{}, fmt.Errorf("plan create: %w", err)
	}
	plan, diff, err := deps.Create(ctx, contract)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan create: %w", err)
	}
	return createReceiptResult(plan, diff)
}

// runGet serves the bounded active view by default and the canonical snapshot
// only on an explicit view=full.
func runGet(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if in.Steps != nil {
		return tooldef.Result{}, errors.New("plan get: takes no steps; use action update or create")
	}
	if in.hasContractFields() {
		return tooldef.Result{}, errors.New("plan get: takes no contract fields; use action create")
	}
	switch in.View {
	case "", "active":
	case "full":
	case "telemetry":
	default:
		return tooldef.Result{}, fmt.Errorf("plan get: unsupported view %q (use active, full, or telemetry)", in.View)
	}
	if in.View == "telemetry" {
		// Diagnostics answer before the plan fetch: observability degrades
		// last, so a failing plan read must not hide the counters (and the
		// snapshot the other views clone is discarded here anyway).
		snapshot, err := deps.Telemetry(ctx)
		if err != nil {
			return tooldef.Result{}, fmt.Errorf("plan get: %w", err)
		}
		return telemetryResult(snapshot)
	}
	plan, err := deps.Get(ctx)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan get: %w", err)
	}
	if in.View == "full" {
		return fullResult(plan)
	}
	return activeViewResult(plan)
}

// runPatch routes an atomic op batch to the session patch engine. The tool
// owns only the routing contract: revision and ops belong to patch, and every
// other payload belongs to create, get, or update — misrouted fields are
// refused rather than silently dropped.
func runPatch(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if in.View != "" {
		return tooldef.Result{}, errors.New("plan patch: view is only valid with action get")
	}
	if in.Steps != nil {
		return tooldef.Result{}, errors.New("plan patch: takes no steps; use action update or create")
	}
	if in.hasContractFields() {
		return tooldef.Result{}, errors.New("plan patch: takes no top-level contract fields; patch ops carry them")
	}
	if in.ExpectedRevision == nil {
		return tooldef.Result{}, errors.New("plan patch: expected_revision is required")
	}
	if in.Ops == nil {
		return tooldef.Result{}, errors.New("plan patch: ops is required")
	}
	for _, op := range in.Ops {
		if actionsCarryRuns(op.Actions.Value) || (op.Step != nil && actionsCarryRuns(op.Step.Actions)) {
			return tooldef.Result{}, errors.New(
				"plan patch: actions take no runs; the harness records them from executed transitions",
			)
		}
	}
	if err := deps.validatePatchPins(in.Ops); err != nil {
		return tooldef.Result{}, fmt.Errorf("plan patch: %w", err)
	}
	plan, summary, err := deps.Patch(ctx, *in.ExpectedRevision, in.Ops)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan patch: %w", err)
	}
	return patchReceiptResult(plan, summary, len(in.Ops))
}

// runTransition routes one lifecycle action to the session state machine. The
// tool owns only the routing contract: id and mutationId belong here, every
// action-specific requirement belongs to the session, and misrouted fields
// are refused rather than silently dropped.
func runTransition(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if in.View != "" {
		return tooldef.Result{}, errors.New("plan transition: view is only valid with action get")
	}
	if in.Steps != nil {
		return tooldef.Result{}, errors.New("plan transition: takes no steps; use action update or create")
	}
	if in.hasContractFields() {
		return tooldef.Result{}, errors.New("plan transition: takes no contract fields; use action create")
	}
	if in.ExpectedRevision != nil {
		return tooldef.Result{}, errors.New("plan transition: takes no expected_revision; use action patch")
	}
	if in.Ops != nil {
		return tooldef.Result{}, errors.New("plan transition: takes no ops; use action patch")
	}
	// A reopen without id addresses the closed plan itself; every other
	// lifecycle action needs its step.
	if in.ID == "" && in.Action != session.TransitionReopen {
		return tooldef.Result{}, errors.New("plan transition: id is required")
	}
	if in.MutationID == "" {
		return tooldef.Result{}, errors.New("plan transition: mutationId is required")
	}
	plan, result, err := deps.Transition(ctx, session.PlanTransition{
		Action:           in.Action,
		StepID:           in.ID,
		MutationID:       in.MutationID,
		Outcome:          in.Outcome,
		Evidence:         in.Evidence,
		EvidenceRefs:     in.EvidenceRefs,
		NoEvidenceReason: in.NoEvidenceReason,
		Blocker:          in.Blocker,
		ResumeWhen:       in.ResumeWhen,
		Reason:           in.Reason,
		PlanResult:       session.PlanResult(in.PlanResult),
	})
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan transition: %w", err)
	}
	return transitionReceiptResult(plan, result)
}

// transitionReceipt is the delta answer to a lifecycle action: what moved and
// the revision the move produced — never the full snapshot.
type transitionReceipt struct {
	Action   string `json:"action"`
	StepID   string `json:"stepId"`
	From     string `json:"from"`
	To       string `json:"to"`
	Revision uint64 `json:"revision"`
	Approved bool   `json:"approved"`
	Replayed bool   `json:"replayed,omitempty"`
	// PlanClosed names the plan-level result this write also recorded.
	PlanClosed string `json:"planClosed,omitempty"`
}

func transitionReceiptResult(plan session.Plan, result session.PlanTransitionResult) (tooldef.Result, error) {
	detail := fmt.Sprintf("%s step %s", result.Action, result.StepID)
	if result.Replayed {
		detail += " (replayed)"
	}
	if result.PlanClosed != "" {
		detail += fmt.Sprintf(", plan closed (%s)", result.PlanClosed)
	}
	return marshalResult(transitionReceipt{
		Action:     result.Action,
		StepID:     result.StepID,
		From:       string(result.From),
		To:         string(result.To),
		Revision:   result.Revision,
		Approved:   plan.Approved,
		Replayed:   result.Replayed,
		PlanClosed: string(result.PlanClosed),
	}, detail)
}

// runUpdate keeps the legacy steps-only replacement on a marked path.
func runUpdate(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if hasNonDefaultView(in.View) {
		return tooldef.Result{}, errors.New("plan update: view is only valid with action get")
	}
	if in.hasNonDefaultContractFields() || stepsCarryV2Fields(in.Steps) {
		return tooldef.Result{}, errors.New("plan update is steps-only; send the v2 contract with action create")
	}
	// Legacy update metadata is tolerated; only steps carry authority.
	if in.Steps == nil {
		return tooldef.Result{}, errors.New("plan: steps is required (use [] to clear)")
	}
	plan, err := deps.Update(ctx, in.Steps)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan update: %w", err)
	}
	return snapshotResult(plan)
}

// Hint returns the plan presence marker for the inference prompt. It is a
// constant string by design: it rides the tail of the system prompt, and a
// per-write change there (revision counters, step counts, approval flips)
// breaks the provider's prefix cache at that point — orphaning the entire
// conversation history after the system prompt, so every plan write re-billed
// the whole context. Volatile plan state (step posture, revisions, attempt
// receipts) reaches the model through tool results and plan tool responses,
// which persist in history and keep the cache prefix intact. The action
// cookbook lives in the plan tool's own schema, not here.
func Hint(plan session.Plan) string {
	if len(plan.Items) == 0 {
		return ""
	}
	if plan.Result != "" {
		return "The durable plan is finished (" + string(plan.Result) + ") and no longer gates tool calls; " +
			"create a new plan, or reopen it with reason to keep working under one."
	}
	return "A durable plan governs this session: gated tool calls must name the step they advance via plan_step; " +
		"the harness starts a pending step for you. Step posture, revisions and attempt receipts arrive in tool " +
		"results and plan tool responses; the plan tool's result carries the authoritative snapshot."
}

func detailFromArgs(raw json.RawMessage) string {
	var in input
	if json.Unmarshal(raw, &in) != nil {
		return "invalid plan"
	}
	switch in.Action {
	case "create":
		return fmt.Sprintf("create %d steps", len(in.Steps))
	case "get":
		if in.View == "full" {
			return "get full"
		}
		return "get active"
	case "patch":
		return fmt.Sprintf("patch %d ops", len(in.Ops))
	case session.TransitionStart, session.TransitionComplete, session.TransitionBlock,
		session.TransitionResume, session.TransitionCancel, session.TransitionReopen:
		if in.ID == "" {
			return in.Action + " plan"
		}
		return fmt.Sprintf("%s step %s", in.Action, in.ID)
	default:
		return fmt.Sprintf("update %d steps", len(in.Steps))
	}
}

// createReceipt is the short answer to action create: revision, draft state,
// progress, and the material diff against the previous plan — enough to
// orient without echoing the contract back.
type createReceipt struct {
	Action   string                       `json:"action"`
	Revision uint64                       `json:"revision"`
	Approved bool                         `json:"approved"`
	Steps    receiptSteps                 `json:"steps"`
	Diff     []session.PlanMaterialChange `json:"diff,omitempty"`
}

func createReceiptResult(plan session.Plan, diff []session.PlanMaterialChange) (tooldef.Result, error) {
	var receipt createReceipt
	receipt.Action = "create"
	receipt.Revision = plan.Revision
	receipt.Approved = plan.Approved
	receipt.Steps.Total = len(plan.Items)
	receipt.Steps.Remaining = remainingSteps(plan.Items)
	receipt.Diff = diff
	detail := fmt.Sprintf("revision %d, %d steps", plan.Revision, receipt.Steps.Total)
	detail += materialChangeSuffix(len(diff))
	return marshalResult(receipt, detail)
}

// patchReceipt is the delta answer to action patch: revision, gate state,
// progress, and what changed — never the full snapshot. The changed block
// carries the material diff that decided approval.
type patchReceipt struct {
	Action   string                   `json:"action"`
	Revision uint64                   `json:"revision"`
	Approved bool                     `json:"approved"`
	Steps    receiptSteps             `json:"steps"`
	Changed  session.PlanPatchSummary `json:"changed"`
}

// receiptSteps is the shared progress block of the create and patch answers.
type receiptSteps struct {
	Total     int `json:"total"`
	Remaining int `json:"remaining"`
}

// materialChangeSuffix names the approval-relevant part of a change count for
// the one-line transcript detail.
func materialChangeSuffix(count int) string {
	if count == 0 {
		return ""
	}
	noun := "material changes"
	if count == 1 {
		noun = "material change"
	}
	return fmt.Sprintf(", %d %s", count, noun)
}

func patchReceiptResult(plan session.Plan, summary session.PlanPatchSummary, opCount int) (tooldef.Result, error) {
	receipt := patchReceipt{
		Action:   "patch",
		Revision: plan.Revision,
		Approved: plan.Approved,
		Steps: receiptSteps{
			Total:     len(plan.Items),
			Remaining: remainingSteps(plan.Items),
		},
		Changed: summary,
	}
	detail := fmt.Sprintf("revision %d, %d ops", plan.Revision, opCount)
	detail += materialChangeSuffix(len(summary.Diff))
	return marshalResult(receipt, detail)
}

// remainingSteps counts steps that are neither completed nor cancelled.
func remainingSteps(items []session.PlanItem) int {
	remaining := 0
	for _, item := range items {
		if item.Status != session.PlanCompleted && item.Status != session.PlanCancelled {
			remaining++
		}
	}
	return remaining
}

// activeView is the bounded default answer to action get: the shared
// projection the plan tool itself returns, wrapped in the tool envelope.
// One renderer, so the authoritative snapshot and the fetched plan can
// never disagree about what matters.
type activeView struct {
	Action string `json:"action"`
	View   string `json:"view"`
	plangate.Projection
}

func activeViewResult(plan session.Plan) (tooldef.Result, error) {
	return marshalResult(activeView{
		Action:     "get",
		View:       "active",
		Projection: plangate.Project(plan),
	}, "get active")
}

// fullResult returns the canonical snapshot verbatim — the same shape the
// session persists — because view=full is an explicit ask for the whole truth.
func fullResult(plan session.Plan) (tooldef.Result, error) {
	return marshalResult(plan, fmt.Sprintf("full snapshot revision %d", plan.Revision))
}

// telemetryResult renders the bounded observability snapshot: counters and
// durations only. By construction it carries no plan text, evidence or
// secret — the schema is uint64 fields end to end.
func telemetryResult(snapshot plantel.Snapshot) (tooldef.Result, error) {
	return marshalResult(snapshot, "get telemetry")
}

func snapshotResult(plan session.Plan) (tooldef.Result, error) {
	items := plan.Items
	if items == nil {
		items = []session.PlanItem{}
	}
	return marshalResult(snapshot{
		Revision:      plan.Revision,
		Approved:      plan.Approved,
		Items:         items,
		Compatibility: "steps-only",
	}, fmt.Sprintf("revision %d, %d steps", plan.Revision, len(items)))
}

// marshalResult encodes one response payload for every action.
func marshalResult(payload any, detail string) (tooldef.Result, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("encode plan response: %w", err)
	}
	content := string(body)
	return tooldef.Result{
		Content: content,
		Detail:  detail,
		Output:  content,
	}, nil
}
