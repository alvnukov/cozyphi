// Package plantool exposes the durable session plan to the primary model.
package plantool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
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
	StepTypes  []string
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
}

// hasTransitionFields reports whether any lifecycle payload rode along. The
// fields belong to the transition actions only; every other action refuses
// them instead of silently dropping work the model believes it sent.
func (in input) hasTransitionFields() bool {
	return in.ID != "" || in.MutationID != "" || in.Outcome != "" || in.Evidence != "" ||
		in.EvidenceRefs != nil || in.NoEvidenceReason != "" || in.Blocker != "" ||
		in.ResumeWhen != "" || in.Reason != ""
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
		in.SuccessCriteria != nil || in.Constraints != nil
}

// stepsCarryV2Fields reports whether any step rides v2 contract metadata. The
// legacy path strips these fields durably, so their presence must be refused
// here rather than silently dropped after the model believed it sent them.
func stepsCarryV2Fields(items []session.PlanItem) bool {
	for _, item := range items {
		if item.ID != "" || item.Why != "" || item.DoneWhen != "" ||
			item.Risk != "" || item.Outcome != "" || item.JIT || item.EvidenceRefs != nil ||
			item.Blocker != "" || item.ResumeWhen != "" {
			return true
		}
	}
	return false
}

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
	stepTypes := deps.StepTypes
	if stepTypes == nil {
		// Standalone callers (tests) get the built-in policy instead of a stale
		// literal copy of it; engines always pass the live runtime types.
		stepTypes = plangate.DefaultDefaults().StepTypeNames()
	}

	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan",
			Description: "Create, read, patch, transition, or replace the durable plan. action=create sends the full work contract (goal, approach, successCriteria, steps with id/why/doneWhen) and starts an unapproved draft; action=get returns a compact view of the current plan (view=full returns the canonical snapshot); action=patch atomically applies ops addressed by stable step ids against expected_revision and answers with the changed delta; the lifecycle actions start/complete/block/resume/cancel/reopen move one step by id (complete carries outcome plus evidence or no_evidence_reason, block carries blocker and resume_when, cancel and reopen carry reason) and replay recorded results for a repeated mutationId; action=update replaces the ordered steps only (legacy steps-only shape). The harness owns the revision; in a v2 plan, after create, status moves only through the lifecycle actions.",
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
						"description": "Response shape for action get; default active.",
						"enum":        []string{"active", "full"},
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
						"items":       llm.Object{"type": "string", "maxLength": 256},
					},
					"constraints": llm.Object{
						"type":        "array",
						"description": "Hard limits the plan must respect.",
						"maxItems":    8,
						"items":       llm.Object{"type": "string", "maxLength": 256},
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
									"description": "Specific actionable step; maximum 256 characters.",
									"maxLength":   256,
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
									"description": "Optional concise finding, assumption, or blocker reason; maximum 256 characters.",
									"maxLength":   256,
								},
								"evidence": llm.Object{
									"type":        "string",
									"description": "Optional concise proof or verification result; maximum 256 characters.",
									"maxLength":   256,
								},
								"id": llm.Object{
									"type":        "string",
									"description": "Stable slug identifying this step; required for create.",
									"maxLength":   64,
								},
								"why": llm.Object{
									"type":        "string",
									"description": "Why this step exists; required for create.",
									"maxLength":   256,
								},
								"doneWhen": llm.Object{
									"type":        "string",
									"description": "Observable condition that ends this step; required for create.",
									"maxLength":   256,
								},
								"risk": llm.Object{
									"type":        "string",
									"description": "What could go wrong and the blast radius.",
									"maxLength":   256,
								},
								"jit": llm.Object{
									"type":        "boolean",
									"description": "True when the step is irreversible and needs just-in-time approval.",
								},
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
								"id": llm.Object{
									"type":        "string",
									"description": "update_step / remove_step target step id.",
								},
								"content": llm.Object{
									"type":        "string",
									"maxLength":   256,
									"description": "update_step.",
								},
								"why": llm.Object{
									"type":        "string",
									"maxLength":   256,
									"description": "update_step.",
								},
								"doneWhen": llm.Object{
									"type":        "string",
									"maxLength":   256,
									"description": "update_step.",
								},
								"risk": llm.Object{
									"type":        "string",
									"maxLength":   256,
									"description": "update_step; optional, null clears.",
								},
								"note": llm.Object{
									"type":        "string",
									"maxLength":   256,
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
											"maxLength":   256,
											"description": "Required.",
										},
										"type": llm.Object{
											"type":        "string",
											"enum":        stepTypes,
											"description": "Required.",
										},
										"why": llm.Object{
											"type":        "string",
											"maxLength":   256,
											"description": "Required.",
										},
										"doneWhen": llm.Object{
											"type":        "string",
											"maxLength":   256,
											"description": "Required.",
										},
										"risk": llm.Object{"type": "string", "maxLength": 256},
										"jit":  llm.Object{"type": "boolean"},
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
									"maxLength":   256,
									"description": "add_/remove_ directive text (its identity).",
								},
								"from": llm.Object{
									"type":        "string",
									"maxLength":   256,
									"description": "update_ directive current text.",
								},
								"to": llm.Object{
									"type":        "string",
									"maxLength":   256,
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
						"description": "Lifecycle target step id; required for start/complete/block/resume/cancel/reopen.",
					},
					"mutationId": llm.Object{
						"type":        "string",
						"maxLength":   64,
						"description": "Idempotency key for one lifecycle action; a retry with the same id replays the recorded result.",
					},
					"outcome": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "complete: concise result the step produced; required.",
					},
					"evidence": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "complete: concise proof; required unless evidence_refs or no_evidence_reason is sent.",
					},
					"evidenceRefs": llm.Object{
						"type":        "array",
						"maxItems":    8,
						"description": "complete: bounded artifacts that prove the outcome.",
						"items":       llm.Object{"type": "string", "maxLength": 128},
					},
					"noEvidenceReason": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "complete: why no evidence can exist; only valid without evidence.",
					},
					"blocker": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "block: what blocks the step; required.",
					},
					"resumeWhen": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "block: the condition that unblocks the step; required.",
					},
					"reason": llm.Object{
						"type":        "string",
						"maxLength":   256,
						"description": "cancel / reopen: why; required.",
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
			if !isTransitionAction(in.Action) && in.hasTransitionFields() {
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
	if in.View != "" {
		return tooldef.Result{}, errors.New("plan create: view is only valid with action get")
	}
	if in.Steps == nil {
		return tooldef.Result{}, errors.New("plan create: steps is required")
	}
	plan, diff, err := deps.Create(ctx, session.PlanV2{
		Goal:            in.Goal,
		Approach:        in.Approach,
		SuccessCriteria: in.SuccessCriteria,
		Constraints:     in.Constraints,
		WorkingContext:  in.WorkingContext,
		Items:           in.Steps,
	})
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
	default:
		return tooldef.Result{}, fmt.Errorf("plan get: unsupported view %q (use active or full)", in.View)
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
	if in.ID == "" {
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
}

func transitionReceiptResult(plan session.Plan, result session.PlanTransitionResult) (tooldef.Result, error) {
	detail := fmt.Sprintf("%s step %s", result.Action, result.StepID)
	if result.Replayed {
		detail += " (replayed)"
	}
	return marshalResult(transitionReceipt{
		Action:   result.Action,
		StepID:   result.StepID,
		From:     string(result.From),
		To:       string(result.To),
		Revision: result.Revision,
		Approved: plan.Approved,
		Replayed: result.Replayed,
	}, detail)
}

// runUpdate keeps the legacy steps-only replacement on a marked path.
func runUpdate(ctx context.Context, deps Deps, in input) (tooldef.Result, error) {
	if in.View != "" {
		return tooldef.Result{}, errors.New("plan update: view is only valid with action get")
	}
	if in.hasContractFields() || stepsCarryV2Fields(in.Steps) {
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

// Hint returns bounded plan metadata for the inference prompt. It deliberately
// excludes model-authored text so a large plan is fetched only when needed and
// cannot smuggle instructions into the system prompt.
func Hint(plan session.Plan) string {
	if len(plan.Items) == 0 {
		return ""
	}
	remaining := remainingSteps(plan.Items)
	state := "unapproved"
	if plan.Approved {
		state = "approved"
	}
	return fmt.Sprintf(
		"Current durable plan: revision %d; %d steps; %d remaining; %s. Create a new contract with "+
			"plan {\"action\":\"create\",...}, adjust it in place with "+
			"plan {\"action\":\"patch\",\"expected_revision\":%d,\"ops\":[...]}, move one step with "+
			"plan {\"action\":\"complete\",\"id\":\"step-id\",\"mutationId\":\"unique-key\",...} "+
			"(start, block, resume, cancel, reopen too), replace the ordered steps only with "+
			"plan {\"action\":\"update\",\"steps\":[...]}; the current snapshot is authoritative. "+
			"Fetch details with plan {\"action\":\"get\"}.",
		plan.Revision,
		len(plan.Items),
		remaining,
		state,
		plan.Revision,
	)
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

// stepSummary is one step as the compact view shows it: identity, the work,
// and what ends it — never the full prose.
type stepSummary struct {
	ID       string `json:"id,omitempty"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Type     string `json:"type,omitempty"`
	DoneWhen string `json:"doneWhen,omitempty"`
	Note     string `json:"note,omitempty"`
}

// blockerSummary names a blocked step, why it waits, and what unblocks it.
type blockerSummary struct {
	ID         string `json:"id,omitempty"`
	Content    string `json:"content"`
	Blocker    string `json:"blocker,omitempty"`
	ResumeWhen string `json:"resumeWhen,omitempty"`
	Note       string `json:"note,omitempty"`
}

// activeView is the bounded default answer to action get. It is bounded by
// construction: every field it copies is capped by the durable schema, and it
// never carries approach, working context, evidence, or completed prose.
type activeView struct {
	Action      string           `json:"action"`
	View        string           `json:"view"`
	Revision    uint64           `json:"revision"`
	Approved    bool             `json:"approved"`
	Goal        string           `json:"goal,omitempty"`
	Constraints []string         `json:"constraints,omitempty"`
	Active      *stepSummary     `json:"active,omitempty"`
	Next        *stepSummary     `json:"next,omitempty"`
	Blockers    []blockerSummary `json:"blockers,omitempty"`
}

func activeViewResult(plan session.Plan) (tooldef.Result, error) {
	view := activeView{
		Action:      "get",
		View:        "active",
		Revision:    plan.Revision,
		Approved:    plan.Approved,
		Goal:        plan.Goal,
		Constraints: plan.Constraints,
	}
	for _, item := range plan.Items {
		switch item.Status {
		case session.PlanInProgress:
			if view.Active == nil {
				summary := stepSummaryOf(item)
				view.Active = &summary
			}
		case session.PlanPending:
			if view.Next == nil {
				summary := stepSummaryOf(item)
				view.Next = &summary
			}
		case session.PlanBlocked:
			view.Blockers = append(view.Blockers, blockerSummary{
				ID:         item.ID,
				Content:    item.Content,
				Blocker:    item.Blocker,
				ResumeWhen: item.ResumeWhen,
				Note:       item.Note,
			})
		}
	}
	return marshalResult(view, "get active")
}

func stepSummaryOf(item session.PlanItem) stepSummary {
	return stepSummary{
		ID:       item.ID,
		Content:  item.Content,
		Status:   string(item.Status),
		Type:     string(item.Type),
		DoneWhen: item.DoneWhen,
		Note:     item.Note,
	}
}

// fullResult returns the canonical snapshot verbatim — the same shape the
// session persists — because view=full is an explicit ask for the whole truth.
func fullResult(plan session.Plan) (tooldef.Result, error) {
	return marshalResult(plan, fmt.Sprintf("full snapshot revision %d", plan.Revision))
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
