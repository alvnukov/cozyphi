// Package questiontool exposes an interactive question tool to the primary
// model: it asks the user to pick from options and returns the chosen labels.
// Ask blocks until the UI answers (or the context/timer dismisses it).
package questiontool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Option is one selectable choice.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Prompt is what the model supplies for one question. The tool schema has no
// `custom` flag (the UI adds a "type your own answer" option by default),
// mirroring opencode's question Prompt.
type Prompt struct {
	Question string   `json:"question"`
	Header   string   `json:"header"`
	Options  []Option `json:"options"`
	Multiple bool     `json:"multiple,omitempty"`
}

// Question is a Prompt plus the `custom` flag the UI honors once rendered.
type Question struct {
	Question string   `json:"question"`
	Header   string   `json:"header"`
	Options  []Option `json:"options"`
	Multiple bool     `json:"multiple,omitempty"`
	Custom   bool     `json:"custom,omitempty"`
}

// Answer holds the selected option labels for one question.
type Answer []string

// Deps binds the tool to the UI ask seam.
type Deps struct {
	Ask func(ctx context.Context, questions []Question) ([]Answer, error)
}

type input struct {
	Questions []Prompt `json:"questions"`
}

// Tool returns the `question` model tool. Its Run blocks until the user has
// answered every question (or the request is dismissed), then hands the answers
// back to the model as a formatted summary.
func Tool(deps Deps) tooldef.Tool {
	if deps.Ask == nil {
		deps.Ask = func(context.Context, []Question) ([]Answer, error) {
			return nil, errors.New("question unavailable")
		}
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "question",
			Description: "Ask the user questions to gather preferences, clarify ambiguous instructions, or get decisions on implementation choices. Answers are returned as arrays of selected option labels; set multiple to allow more than one.",
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"questions": llm.Object{
						"type":        "array",
						"description": "Questions to ask",
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"question": llm.Object{"type": "string", "description": "Complete question"},
								"header": llm.Object{
									"type":        "string",
									"description": "Very short label (max 30 chars)",
								},
								"options": llm.Object{
									"type": "array",
									"items": llm.Object{
										"type": "object",
										"properties": llm.Object{
											"label": llm.Object{
												"type":        "string",
												"description": "Display text (1-5 words, concise)",
											},
											"description": llm.Object{
												"type":        "string",
												"description": "Explanation of choice",
											},
										},
										"required": []string{"label", "description"},
									},
								},
								"multiple": llm.Object{
									"type":        "boolean",
									"description": "Allow selecting multiple choices",
								},
							},
							"required": []string{"question", "header", "options"},
						},
					},
				},
				Required: []string{"questions"},
			},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
			var in input
			if err := tooldef.DecodeStrict(raw, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("question args: %w", err)
			}
			if len(in.Questions) == 0 {
				return tooldef.Result{}, errors.New("question: at least one question is required")
			}
			questions := make([]Question, 0, len(in.Questions))
			for _, p := range in.Questions {
				questions = append(questions, Question{
					Question: p.Question,
					Header:   p.Header,
					Options:  p.Options,
					Multiple: p.Multiple,
					Custom:   true,
				})
			}
			answers, err := deps.Ask(ctx, questions)
			if err != nil {
				return tooldef.Result{}, fmt.Errorf("question: %w", err)
			}
			content := "User has answered your questions: " + formatAnswers(in.Questions, answers) +
				". You can now continue with the user's answers in mind."
			return tooldef.Result{
				Content: content,
				Output:  content,
				Detail:  fmt.Sprintf("asked %d question(s)", len(in.Questions)),
			}, nil
		},
		DetailFromArgs: func(raw json.RawMessage) string {
			var in input
			if json.Unmarshal(raw, &in) != nil {
				return "invalid question"
			}
			return fmt.Sprintf("asked %d question(s)", len(in.Questions))
		},
	}
}

func formatAnswers(questions []Prompt, answers []Answer) string {
	parts := make([]string, 0, len(questions))
	for i, q := range questions {
		value := "Unanswered"
		if i < len(answers) && len(answers[i]) > 0 {
			value = strings.Join(answers[i], ", ")
		}
		parts = append(parts, fmt.Sprintf("%q=%q", q.Question, value))
	}
	return strings.Join(parts, ", ")
}
