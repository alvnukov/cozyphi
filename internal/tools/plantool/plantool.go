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
	Update    func(context.Context, []session.PlanItem) (session.Plan, error)
	Create    func(context.Context, session.PlanV2) (session.Plan, error)
	Get       func(context.Context) (session.Plan, error)
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
	Action           string             `json:"action"`
	View             string             `json:"view"`
	ExpectedRevision *uint64            `json:"expected_revision"`
	Steps            []session.PlanItem `json:"steps"`
	Goal             string             `json:"goal"`
	Approach         string             `json:"approach"`
	SuccessCriteria  []string           `json:"successCriteria"`
	Constraints      []string           `json:"constraints"`
	WorkingContext   string             `json:"workingContext"`
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
			item.Risk != "" || item.Outcome != "" || item.JIT || item.EvidenceRefs != nil {
			return true
		}
	}
	return false
}

// Tool returns the model-facing interface to the canonical durable plan:
// create sends a full v2 work contract as an unapproved draft, get reads a
// bounded view of the current plan, and update keeps the legacy steps-only
// replacement. The harness owns the revision.
func Tool(deps Deps) tooldef.Tool {
	unavailable := errors.New("session plan unavailable")
	if deps.Update == nil {
		deps.Update = func(context.Context, []session.PlanItem) (session.Plan, error) {
			return session.Plan{}, unavailable
		}
	}
	if deps.Create == nil {
		deps.Create = func(context.Context, session.PlanV2) (session.Plan, error) {
			return session.Plan{}, unavailable
		}
	}
	if deps.Get == nil {
		deps.Get = func(context.Context) (session.Plan, error) {
			return session.Plan{}, unavailable
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
			Description: "Create, read, or replace the durable plan. action=create sends the full work contract (goal, approach, successCriteria, steps with id/why/doneWhen) and starts an unapproved draft; action=get returns a compact view of the current plan (view=full returns the canonical snapshot); action=update replaces the ordered steps only (legacy steps-only shape). The harness owns the revision.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type":        "string",
						"description": "Discriminates the call: create sends the full work contract, get reads the current plan, update replaces the ordered steps only (legacy).",
						"enum":        []string{"create", "get", "update"},
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
			switch in.Action {
			case "create":
				return runCreate(ctx, deps, in)
			case "get":
				return runGet(ctx, deps, in)
			case "", "update":
				return runUpdate(ctx, deps, in)
			default:
				return tooldef.Result{}, fmt.Errorf(
					"plan: unsupported action %q (use create, get, or update)", in.Action,
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
	plan, err := deps.Create(ctx, session.PlanV2{
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
	return createReceiptResult(plan)
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
			"plan {\"action\":\"create\",...}, replace the ordered steps only with "+
			"plan {\"action\":\"update\",\"steps\":[...]}; the current snapshot is authoritative. "+
			"Fetch details with plan {\"action\":\"get\"}.",
		plan.Revision,
		len(plan.Items),
		remaining,
		state,
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
	default:
		return fmt.Sprintf("update %d steps", len(in.Steps))
	}
}

// createReceipt is the short answer to action create: revision, draft state,
// and progress — enough to orient without echoing the contract back.
type createReceipt struct {
	Action   string `json:"action"`
	Revision uint64 `json:"revision"`
	Approved bool   `json:"approved"`
	Steps    struct {
		Total     int `json:"total"`
		Remaining int `json:"remaining"`
	} `json:"steps"`
}

func createReceiptResult(plan session.Plan) (tooldef.Result, error) {
	var receipt createReceipt
	receipt.Action = "create"
	receipt.Revision = plan.Revision
	receipt.Approved = plan.Approved
	receipt.Steps.Total = len(plan.Items)
	receipt.Steps.Remaining = remainingSteps(plan.Items)
	return marshalResult(receipt, fmt.Sprintf("revision %d, %d steps", plan.Revision, receipt.Steps.Total))
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

// blockerSummary names a blocked step and why it waits.
type blockerSummary struct {
	ID      string `json:"id,omitempty"`
	Content string `json:"content"`
	Note    string `json:"note,omitempty"`
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
				ID:      item.ID,
				Content: item.Content,
				Note:    item.Note,
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
