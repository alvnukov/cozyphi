// Package plantool exposes the current session plan to the primary model.
package plantool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
)

// Deps binds the model tool to the engine's current session.
type Deps struct {
	Update func(context.Context, []session.PlanItem) (session.Plan, error)
}

// Tool atomically replaces the ordered session plan. A single whole-list
// operation avoids partial CRUD sequences and leaves persistence/event ordering
// inside the session module.
func Tool(deps Deps) tooldef.Tool {
	if deps.Update == nil {
		deps.Update = func(context.Context, []session.PlanItem) (session.Plan, error) {
			return session.Plan{}, errors.New("update_plan: session plan unavailable")
		}
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "update_plan",
			Description: "Create or replace the structured work plan for this session. Use it for non-trivial work with several meaningful steps. Keep at most one step in_progress, update statuses as work happens, and mark completed only after verification. Send the complete ordered list on every call; send an empty list to clear the plan.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"steps": llm.Object{
						"type":        "array",
						"description": "Complete ordered plan snapshot.",
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"content": llm.Object{"type": "string", "description": "Specific actionable step."},
								"status": llm.Object{
									"type": "string",
									"enum": []string{"pending", "in_progress", "completed", "cancelled"},
								},
							},
							"required": []string{"content", "status"},
						},
					},
				},
				Required: []string{"steps"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Steps []session.PlanItem `json:"steps"`
			}
			if json.Unmarshal(input, &in) != nil {
				return "invalid plan"
			}
			return fmt.Sprintf("%d steps", len(in.Steps))
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Steps []session.PlanItem `json:"steps"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("update_plan args: %w", err)
			}
			if in.Steps == nil {
				return tooldef.Result{}, errors.New("update_plan: steps is required (use [] to clear)")
			}
			plan, err := deps.Update(ctx, in.Steps)
			if err != nil {
				return tooldef.Result{}, fmt.Errorf("update_plan: %w", err)
			}
			pending := 0
			for _, item := range plan.Items {
				if item.Status != session.PlanCompleted && item.Status != session.PlanCancelled {
					pending++
				}
			}
			body := fmt.Sprintf(
				"Plan updated: revision %d, %d steps, %d remaining.",
				plan.Revision,
				len(plan.Items),
				pending,
			)
			return tooldef.Result{Content: body, Detail: fmt.Sprintf("%d steps", len(plan.Items)), Output: body}, nil
		},
	}
}
