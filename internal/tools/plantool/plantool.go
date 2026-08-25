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

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
)

// Deps binds the model tool to the engine's current session.
type Deps struct {
	Read   func() session.Plan
	Update func(context.Context, uint64, []session.PlanItem) (session.Plan, error)
}

type snapshot struct {
	Revision uint64             `json:"revision"`
	Approved bool               `json:"approved,omitempty"`
	Items    []session.PlanItem `json:"items"`
}

type input struct {
	Action           string             `json:"action"`
	ExpectedRevision *uint64            `json:"expected_revision"`
	Steps            []session.PlanItem `json:"steps"`
}

// Tool returns one action-based interface to the canonical durable plan.
// Whole-snapshot replacement keeps partial CRUD and merge policy inside the
// model while the revision prevents lost updates.
func Tool(deps Deps) tooldef.Tool {
	if deps.Read == nil {
		deps.Read = func() session.Plan { return session.Plan{} }
	}
	if deps.Update == nil {
		deps.Update = func(context.Context, uint64, []session.PlanItem) (session.Plan, error) {
			return session.Plan{}, errors.New("session plan unavailable")
		}
	}

	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "plan",
			Description: "Read or atomically replace the durable session plan. Use action=get before continuing or updating an existing plan. For action=update, send expected_revision from get and the complete ordered steps list; [] clears it.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type":        "string",
						"description": "Operation to perform.",
						"enum":        []string{"get", "update"},
					},
					"expected_revision": llm.Object{
						"type":        "integer",
						"description": "Revision returned by action=get or the latest update result; required for update.",
						"minimum":     0,
					},
					"steps": llm.Object{
						"type":        "array",
						"description": "Complete ordered plan snapshot for update; maximum 32 steps.",
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
				Required: []string{"action"},
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
				if in.ExpectedRevision != nil || in.Steps != nil {
					return tooldef.Result{}, errors.New(
						"plan get: expected_revision and steps are only valid for action=update",
					)
				}
				return snapshotResult(deps.Read())
			case "update":
				if in.ExpectedRevision == nil {
					return tooldef.Result{}, errors.New(
						"plan update: expected_revision is required; call plan with action=get first",
					)
				}
				if in.Steps == nil {
					return tooldef.Result{}, errors.New("plan update: steps is required (use [] to clear)")
				}
				plan, err := deps.Update(ctx, *in.ExpectedRevision, in.Steps)
				if err != nil {
					return tooldef.Result{}, fmt.Errorf("plan update: %w", err)
				}
				return snapshotResult(plan)
			case "":
				return tooldef.Result{}, errors.New("plan: action is required (get or update)")
			default:
				return tooldef.Result{}, fmt.Errorf("plan: unsupported action %q (use get or update)", in.Action)
			}
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
		"Current durable plan: revision %d; %d steps; %d remaining; %s. Call plan with action=get before continuing or updating it.",
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
	case "get":
		return "get"
	case "update":
		return fmt.Sprintf("update %d steps", len(in.Steps))
	default:
		return "invalid plan"
	}
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
