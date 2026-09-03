// Package plantool exposes the durable session plan to the primary model.
package plantool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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
	// Skills lists the skill catalog names the model may put on a step; nil
	// means no catalog is wired and name validation is off.
	Skills    func() []string
	StepTypes []string
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
	Action           string                `json:"action"`
	View             string                `json:"view"`
	ExpectedRevision *uint64               `json:"expected_revision"`
	Steps            []session.PlanItem    `json:"steps"`
	Goal             string                `json:"goal"`
	Approach         string                `json:"approach"`
	SuccessCriteria  []string              `json:"successCriteria"`
	Constraints      []string              `json:"constraints"`
	WorkingContext   string                `json:"workingContext"`
	Ops              []session.PlanPatchOp `json:"ops"`

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

// scoped makes action the sole discriminator. Some tool clients materialize
// every advertised property, including empty arrays and the first enum value;
// treating those provider-generated values as model intent makes an overloaded
// schema impossible to call reliably. Unknown fields are still rejected by the
// strict decoder, while known fields owned by another action are discarded here.
func (in input) scoped() input {
	switch in.Action {
	case "create":
		return input{
			Action: in.Action, Steps: in.Steps, Goal: in.Goal, Approach: in.Approach,
			SuccessCriteria: in.SuccessCriteria, Constraints: in.Constraints,
			WorkingContext: in.WorkingContext,
		}
	case "get":
		return input{Action: in.Action, View: in.View}
	case "patch":
		// A materialized expected_revision of 0 can never name a stored v2
		// plan (create starts at revision 1), so it is noise, not a CAS intent.
		var revision *uint64
		if in.ExpectedRevision != nil && *in.ExpectedRevision != 0 {
			revision = in.ExpectedRevision
		}
		return input{Action: in.Action, ExpectedRevision: revision, Ops: scopedPatchOps(in.Ops)}
	case session.TransitionStart, session.TransitionComplete, session.TransitionBlock,
		session.TransitionResume, session.TransitionCancel, session.TransitionReopen:
		scoped := input{Action: in.Action, ID: in.ID, MutationID: in.MutationID}
		switch in.Action {
		case session.TransitionComplete:
			scoped.Outcome = in.Outcome
			scoped.Evidence = in.Evidence
			scoped.EvidenceRefs = in.EvidenceRefs
			scoped.NoEvidenceReason = in.NoEvidenceReason
			scoped.PlanResult = in.PlanResult
		case session.TransitionBlock:
			scoped.Blocker = in.Blocker
			scoped.ResumeWhen = in.ResumeWhen
		case session.TransitionCancel, session.TransitionReopen:
			scoped.Reason = in.Reason
		}
		return scoped
	case "", "update":
		return input{Action: in.Action, Steps: in.Steps}
	default:
		return input{Action: in.Action}
	}
}

// scopedPatchOps applies the same discriminator rule inside the nested patch
// union. It deliberately copies PatchValue slots instead of normalizing them:
// for a field owned by the selected op, omitted and explicit JSON null must
// remain distinct. The session layer stays strict for direct callers, while
// provider-materialized fields owned by other ops disappear at the tool seam.
func scopedPatchOps(ops []session.PlanPatchOp) []session.PlanPatchOp {
	if ops == nil {
		return nil
	}
	out := make([]session.PlanPatchOp, len(ops))
	for i, op := range ops {
		scoped := session.PlanPatchOp{Op: op.Op}
		switch op.Op {
		case session.PlanPatchSetPlanFields:
			scoped.Goal = op.Goal
			scoped.Approach = op.Approach
			// Human-only fields ride along so the guard below can refuse the
			// call; deps.Patch never sees them.
			scoped.Actions = op.Actions
			scoped.ModelsByType = op.ModelsByType
		case session.PlanPatchReplaceContext:
			scoped.WorkingContext = op.WorkingContext
		case session.PlanPatchUpdateStep:
			scoped.ID = op.ID
			scoped.Content = op.Content
			scoped.Why = op.Why
			scoped.DoneWhen = op.DoneWhen
			scoped.Risk = op.Risk
			scoped.Note = op.Note
			scoped.Model = op.Model
			scoped.Actions = op.Actions
			scoped.Skills = op.Skills
		case session.PlanPatchInsertStep:
			scoped.Before = op.Before
			scoped.After = op.After
			scoped.Step = op.Step
		case session.PlanPatchSupersedeStep:
			scoped.ID = op.ID
			scoped.Step = op.Step
		case session.PlanPatchRemoveStep:
			scoped.ID = op.ID
		case session.PlanPatchReorderSteps:
			scoped.IDs = op.IDs
		case session.PlanPatchAddConstraint, session.PlanPatchRemoveConstraint,
			session.PlanPatchAddCriterion, session.PlanPatchRemoveCriterion:
			scoped.Value = op.Value
		case session.PlanPatchUpdateConstraint, session.PlanPatchUpdateCriterion:
			scoped.From = op.From
			scoped.To = op.To
		}
		out[i] = scoped
	}
	return out
}

// hasContractFields reports whether any v2 contract field rode along. The
// legacy update path must refuse them instead of silently dropping work the
// model believes it sent.
func (in input) hasContractFields() bool {
	return in.Goal != "" || in.Approach != "" || in.WorkingContext != "" ||
		in.SuccessCriteria != nil || in.Constraints != nil
}

// hasNonDefaultContractFields is the update-path variant: empty arrays are
// provider defaults, not an attempt to mutate the v2 contract.
func (in input) hasNonDefaultContractFields() bool {
	return in.Goal != "" || in.Approach != "" || in.WorkingContext != "" ||
		len(in.SuccessCriteria) > 0 || len(in.Constraints) > 0
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
			item.Model != "" || item.Actions != nil || item.Skills != nil {
			return true
		}
	}
	return false
}

// errHumanOnly is the one answer any model-authored attempt to set the
// plan's automation fields gets. Step model pins, actions and the type map
// are the user's: configured in the plan UI, invisible in the tool schema,
// so their presence in a call is a rogue or stale client, not intent.
var errHumanOnly = errors.New(
	`plan: "model", "actions" and "modelsByType" are human-only; the plan UI owns them`,
)

// stepsCarryHumanOnlyFields reports whether model-authored steps ride the
// human-only fields; create refuses them next to the attempts guard.
func stepsCarryHumanOnlyFields(items []session.PlanItem) bool {
	for _, item := range items {
		if item.Model != "" || item.Actions != nil {
			return true
		}
	}
	return false
}

// opsCarryHumanOnlyFields reports whether a patch op the discriminator would
// honor carries a human-only field. Fields owned by another op are dropped by
// scoped() as usual; a field this op owns must be refused, not silently kept.
func opsCarryHumanOnlyFields(ops []session.PlanPatchOp) bool {
	for _, op := range ops {
		switch op.Op {
		case session.PlanPatchSetPlanFields:
			if op.Actions.Set || op.ModelsByType.Set {
				return true
			}
		case session.PlanPatchUpdateStep:
			if op.Model.Set || op.Actions.Set {
				return true
			}
		case session.PlanPatchInsertStep:
			if op.Step != nil && (op.Step.Model != "" || op.Step.Actions != nil) {
				return true
			}
		}
	}
	return false
}

// modelVisibleDiff drops the human-only entries from a material diff bound
// for a model-facing receipt. The sidebar renders the unfiltered diff for
// the user; the model never reads the user's model and automation choices,
// not even as a diff line.
func modelVisibleDiff(diff []session.PlanMaterialChange) []session.PlanMaterialChange {
	if diff == nil {
		return nil
	}
	out := make([]session.PlanMaterialChange, 0, len(diff))
	for _, change := range diff {
		if change.Field == "model" || change.Field == "modelsByType" {
			continue
		}
		if change.Field == "actions" && !strings.Contains(change.Detail, string(session.PlanActionInjectSkill)) {
			// Skills are the model's own automation; every other action
			// change is the user's configuration and stays hidden.
			continue
		}
		out = append(out, change)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizePlanForModel copies a plan for a model-facing response and strips
// the human-owned settings: per-step model pins and actions, the type map
// and plan-level actions. The inject_skill lists survive — the model
// authored them — but their runs (harness audit) drop with everything else.
// The user's TUI renders the real snapshot; tool responses answer the model.
func sanitizePlanForModel(plan session.Plan) session.Plan {
	out := plan
	out.Actions = nil
	out.ModelsByType = nil
	out.Items = append([]session.PlanItem(nil), plan.Items...)
	for i := range out.Items {
		out.Items[i].Model = ""
		out.Items[i].Actions = modelOwnedActions(out.Items[i].Actions)
	}
	return out
}

// modelOwnedActions keeps only the inject_skill actions, minus runs: the
// skills list is the model's own authoring and stays visible in every view,
// while every other action (and every run record) is user configuration or
// harness audit the model never sees.
func modelOwnedActions(actions []session.PlanAction) []session.PlanAction {
	var kept []session.PlanAction
	for _, action := range actions {
		if action.Type != session.PlanActionInjectSkill {
			continue
		}
		kept = append(kept, session.PlanAction{
			Event:          action.Event,
			Type:           action.Type,
			Skills:         action.Skills,
			DisabledSkills: action.DisabledSkills,
		})
	}
	return kept
}

// hasNonDefaultView reports a real response-shape override. Some providers
// materialize the advertised "active" default for every action; it remains
// semantically absent outside get, while "full" must still be rejected.
func hasNonDefaultView(view string) bool {
	return view != "" && view != "active"
}

// Tool returns the model-facing interface to the canonical durable plan:
// create sends a full v2 work contract as an unapproved draft, get reads a
// bounded view of the current plan, patch atomically applies domain-specific
// operations against the harness-owned current revision (or an explicit CAS),
// the lifecycle actions move one step through the validated state machine, and
// update keeps the legacy steps-only replacement. In a v2 plan, after create,
// status moves only through the lifecycle actions.
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
	// The catalog names ride the schema so the planner picks each step's
	// skills at the decision point instead of guessing spellings; validation
	// still refuses unknown names at the seam. No catalog wired, no clause —
	// the schema stays byte-identical for catalog-less callers.
	stepSkillsDesc := "Recommended skills for this step, injected at step start. Absent inherits the step-type defaults; an explicit list replaces them; an explicit empty list removes the injection."
	updateSkillsDesc := "update_step: replace the step's injected skills. An explicit list replaces the step-type defaults; an explicit empty list or null removes the injection; omit to keep."
	if deps.Skills != nil {
		if names := deps.Skills(); len(names) > 0 {
			catalog := " Pick from the installed skill catalog, matching each step's content: " +
				strings.Join(names, ", ") + "."
			stepSkillsDesc += catalog
			updateSkillsDesc += catalog
		}
	}

	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan",
			Description: "Create, read, patch, transition, or replace the durable plan. action=create sends the full work contract (goal, approach, successCriteria, steps with id/why/doneWhen) and starts an unapproved draft; action=get returns the compact projection (view=full returns the canonical snapshot with audit history); action=patch atomically applies ops addressed by stable step ids under session compare-and-swap and answers with the changed delta (expected_revision is optional and requests an explicit revision check); the lifecycle actions start/complete/block/resume/cancel/reopen move one step by id (complete carries outcome plus evidence or noEvidenceReason, and a call:<callId> evidence ref must cite a recorded successful attempt, block carries blocker and resumeWhen, cancel and reopen carry reason) and replay recorded results using mutationId or harness-derived tool-call identity; complete with planResult also closes the finished plan in the same write (the bounded terminal view then replaces the projection), and reopen without id restores a closed plan; action=update replaces the ordered steps only (legacy steps-only shape). The harness owns the revision; in a v2 plan, after create, status moves only through the lifecycle actions. Plan prose holds concise operational rationale — never secrets, raw logs, or raw chain-of-thought; cite bounded evidence refs instead.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type":        "string",
						"description": "Discriminates the call: create sends the full work contract, get reads the current plan, patch applies an atomic op batch under session CAS, start/complete/block/resume/cancel/reopen move one step through the lifecycle, update replaces the ordered steps only (legacy).",
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
						"description": "Optional compare-and-swap revision for action patch; omit to use the harness-owned current revision.",
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
								"skills": llm.Object{
									"type":        "array",
									"maxItems":    8,
									"items":       llm.Object{"type": "string", "maxLength": 64},
									"description": stepSkillsDesc,
								},
							},
							"required": []string{"content", "type"},
						},
					},
					"ops": llm.Object{
						// The bounds below mirror the session layer's unexported
						// budget (internal/session/plan.go); the golden definition
						// test pins them so drift is caught in review.
						"type":        "array",
						"description": "Atomic patch batch for action patch; maximum 32 ops, applied all-or-none under session CAS. Each op reads only its own fields; scalar slots: absent keeps the value, a value replaces it, JSON null clears an optional one.",
						"maxItems":    32,
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"op": llm.Object{
									"type": "string",
									"enum": []string{
										"set_plan_fields", "replace_context", "update_step", "insert_step",
										"remove_step", "supersede_step", "reorder_steps",
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
								"id": llm.Object{
									"type":        "string",
									"description": "update_step / remove_step / supersede_step target step id.",
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
								"skills": llm.Object{
									"type":        "array",
									"maxItems":    8,
									"items":       llm.Object{"type": "string", "maxLength": 64},
									"description": updateSkillsDesc,
								},
								"before": llm.Object{
									"type":        "string",
									"description": "insert_step anchor: place the new step before this id; one anchor is required unless the plan has no steps yet.",
								},
								"after": llm.Object{
									"type":        "string",
									"description": "insert_step anchor: place the new step after this id; one anchor is required unless the plan has no steps yet.",
								},
								"step": llm.Object{
									"type":        "object",
									"description": "insert_step / supersede_step replacement; starts pending.",
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
										"risk": llm.Object{"type": "string", "maxLength": 512},
										"jit":  llm.Object{"type": "boolean"},
										"skills": llm.Object{
											"type":        "array",
											"maxItems":    8,
											"items":       llm.Object{"type": "string", "maxLength": 64},
											"description": stepSkillsDesc,
										},
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
						"description": "Optional idempotency key for a lifecycle retry; the harness derives it from the tool call when omitted.",
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
						"description": "complete: optionally close the whole plan as success or abandoned when this is the last active step. Refused while work remains. Omit to complete only the step.",
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
			in = in.scoped()
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
	}
	if stepsCarryHumanOnlyFields(in.Steps) {
		return tooldef.Result{}, fmt.Errorf("plan create: %w", errHumanOnly)
	}
	for _, item := range in.Steps {
		if err := validateSkillNames(deps, item.Skills, "create"); err != nil {
			return tooldef.Result{}, err
		}
	}
	contract := session.PlanV2{
		Goal:            in.Goal,
		Approach:        in.Approach,
		SuccessCriteria: in.SuccessCriteria,
		Constraints:     in.Constraints,
		WorkingContext:  in.WorkingContext,
		Items:           in.Steps,
	}
	plan, diff, err := deps.Create(ctx, contract)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan create: %w", err)
	}
	return createReceiptResult(plan, diff)
}

// validateSkillNames refuses skill names the catalog does not know, so a
// typo fails at the seam instead of silently injecting nothing at step
// start. With no catalog wired there is nothing to check against.
func validateSkillNames(deps Deps, names []string, what string) error {
	if deps.Skills == nil || len(names) == 0 {
		return nil
	}
	known := make(map[string]struct{})
	for _, name := range deps.Skills() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("plan %s: unknown skill %q; use names from the skill catalog", what, name)
		}
	}
	return nil
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

// runPatch validates the selected patch payload. When the model omits the
// revision, the harness snapshots its current value and the session's atomic
// compare-and-swap remains the concurrency backstop.
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
	if in.Ops == nil {
		return tooldef.Result{}, errors.New("plan patch: ops is required")
	}
	if opsCarryHumanOnlyFields(in.Ops) {
		return tooldef.Result{}, fmt.Errorf("plan patch: %w", errHumanOnly)
	}
	for _, op := range in.Ops {
		if err := validateSkillNames(deps, op.Skills.Value, "patch"); err != nil {
			return tooldef.Result{}, err
		}
		if op.Step != nil {
			if err := validateSkillNames(deps, op.Step.Skills, "patch"); err != nil {
				return tooldef.Result{}, err
			}
		}
	}
	revision := in.ExpectedRevision
	if revision == nil {
		current, err := deps.Get(ctx)
		if err != nil {
			return tooldef.Result{}, fmt.Errorf("plan patch: read current revision: %w", err)
		}
		revision = &current.Revision
	}
	plan, summary, err := deps.Patch(ctx, *revision, in.Ops)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("plan patch: %w", err)
	}
	return patchReceiptResult(plan, summary, len(in.Ops))
}

// runTransition routes one lifecycle action to the session state machine. The
// action selects the payload, and the harness derives idempotency from the tool
// call when the model omits mutationId.
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
	mutationID := in.MutationID
	if mutationID == "" {
		mutationID = mutationIDFromContext(ctx)
	}
	if mutationID == "" {
		return tooldef.Result{}, errors.New("plan transition: mutationId is required outside a harness tool call")
	}
	plan, result, err := deps.Transition(ctx, session.PlanTransition{
		Action:           in.Action,
		StepID:           in.ID,
		MutationID:       mutationID,
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

func mutationIDFromContext(ctx context.Context) string {
	callID := tooldef.ToolCallID(ctx)
	if callID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(callID))
	return fmt.Sprintf("call-%x", sum[:12])
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
	visible := modelVisibleDiff(diff)
	receipt.Diff = visible
	detail := fmt.Sprintf("revision %d, %d steps", plan.Revision, receipt.Steps.Total)
	detail += materialChangeSuffix(len(visible))
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
	// The receipt is model-facing: human-only diff lines (model pins,
	// actions, the type map) never reach it, even second-hand.
	summary.Diff = modelVisibleDiff(summary.Diff)
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

// remainingSteps counts steps that still owe work.
func remainingSteps(items []session.PlanItem) int {
	remaining := 0
	for _, item := range items {
		if !item.Status.Terminal() {
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

// fullResult returns the canonical snapshot minus the human-owned settings:
// view=full is an explicit ask for the whole truth the model may act on, and
// step model pins, actions and the type map are not the model's business.
// The user's TUI renders the untouched snapshot.
func fullResult(plan session.Plan) (tooldef.Result, error) {
	return marshalResult(sanitizePlanForModel(plan), fmt.Sprintf("full snapshot revision %d", plan.Revision))
}

// telemetryResult renders the bounded observability snapshot: counters and
// durations only. By construction it carries no plan text, evidence or
// secret — the schema is uint64 fields end to end.
func telemetryResult(snapshot plantel.Snapshot) (tooldef.Result, error) {
	return marshalResult(snapshot, "get telemetry")
}

func snapshotResult(plan session.Plan) (tooldef.Result, error) {
	items := sanitizePlanForModel(plan).Items
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
