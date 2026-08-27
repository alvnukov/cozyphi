// Package plantool exposes the durable session plan to the primary model.
package plantool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Deps binds the model tool to the engine's current session.
type Deps struct {
	Update func(context.Context, []session.PlanItem) (session.Plan, error)
}

type snapshot struct {
	Revision uint64             `json:"revision"`
	Approved bool               `json:"approved"`
	Items    []session.PlanItem `json:"items"`
}

// input decodes the steps-only contract. The legacy action and expected_revision
// fields stay recognized so a resumed session that replays an old update call
// does not trip strict decoding; they carry no authority and are dropped on read.
type input struct {
	Action           string             `json:"action"`
	ExpectedRevision *uint64            `json:"expected_revision"`
	Steps            []session.PlanItem `json:"steps"`
}

// Tool returns the steps-only interface to the canonical durable plan. The model
// supplies the whole ordered step list; the harness replaces the current plan
// atomically under one lock, so there is no model-supplied revision to compare.
func Tool(deps Deps) tooldef.Tool {
	if deps.Update == nil {
		deps.Update = func(context.Context, []session.PlanItem) (session.Plan, error) {
			return session.Plan{}, errors.New("session plan unavailable")
		}
	}

	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan",
			Description: "Atomically replace the current plan with the supplied steps. Send the complete ordered steps list; [] clears it. The harness owns the revision.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
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
									"enum": []string{"pending", "in_progress", "blocked", "completed", "cancelled"},
								},
								"type": llm.Object{
									"type":        "string",
									"description": "What this step is allowed to do; empty means any tool.",
									"enum":        []string{"explore", "edit", "run", "delegate", "integrate"},
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
							},
							"required": []string{"content", "status"},
						},
					},
				},
				Required: []string{"steps"},
			},
		},
		DetailFromArgs: detailFromArgs,
		Run: func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
			var in input
			if err := decodeStrict(raw, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("plan args: %w", err)
			}
			switch in.Action {
			case "get":
				return tooldef.Result{}, errors.New(
					"plan get is no longer supported: the current plan is injected into the context on every inference",
				)
			case "", "update":
				// Legacy update metadata is tolerated; only steps carry authority.
			default:
				return tooldef.Result{}, fmt.Errorf("plan: unsupported action %q (use plan with steps only)", in.Action)
			}
			if in.Steps == nil {
				return tooldef.Result{}, errors.New("plan: steps is required (use [] to clear)")
			}
			plan, err := deps.Update(ctx, in.Steps)
			if err != nil {
				return tooldef.Result{}, fmt.Errorf("plan update: %w", err)
			}
			return snapshotResult(plan)
		},
	}
}

// Hint returns bounded plan metadata for the inference prompt. It deliberately
// excludes model-authored text so a large plan is fetched only when needed and
// cannot smuggle instructions into the system prompt.
func Hint(plan session.Plan) string {
	if len(plan.Items) == 0 {
		return ""
	}
	remaining := 0
	for _, item := range plan.Items {
		if item.Status != session.PlanCompleted && item.Status != session.PlanCancelled {
			remaining++
		}
	}
	state := "unapproved"
	if plan.Approved {
		state = "approved"
	}
	return fmt.Sprintf(
		"Current durable plan: revision %d; %d steps; %d remaining; %s. Replace it with plan {\"steps\":[...]}; the current snapshot is authoritative.",
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
	if in.Action == "get" {
		return "get"
	}
	return fmt.Sprintf("update %d steps", len(in.Steps))
}

func snapshotResult(plan session.Plan) (tooldef.Result, error) {
	items := plan.Items
	if items == nil {
		items = []session.PlanItem{}
	}
	body, err := json.Marshal(snapshot{Revision: plan.Revision, Approved: plan.Approved, Items: items})
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("encode plan snapshot: %w", err)
	}
	content := string(body)
	return tooldef.Result{
		Content: content,
		Detail:  fmt.Sprintf("revision %d, %d steps", plan.Revision, len(items)),
		Output:  content,
	}, nil
}

func decodeStrict(raw json.RawMessage, dst any) error {
	if strings.TrimSpace(string(raw)) == "" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
