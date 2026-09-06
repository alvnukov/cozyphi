// Package sessiontool names the owning primary session without another model request.
package sessiontool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Instruction is shared by the tool description and the primary-session prompt.
const Instruction = `When the session goal becomes clear (usually in the first turn), call session with action=set_title. Use 3–7 words in the user's language, without quotes or a trailing period; never include secrets. Update only when the topic substantially changes. A title set by /rename is pinned: do not override it. Use the normal tool loop, not a separate model request.`

// Tool binds naming to one session. setTitle validates and durably stores the
// model title, atomically respecting the user's pin, and returns its saved form.
func Tool(setTitle func(string) (string, error)) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "session", Description: "Name the current primary session. " + Instruction,
			Params: &llm.FunctionParameters{Type: "object", Properties: llm.Object{
				"action": llm.Object{"type": "string", "enum": []string{"set_title"}},
				"title": llm.Object{
					"type":        "string",
					"description": "Short session title in the user's language (1–60 characters).",
				},
			}, Required: []string{"action", "title"}},
		},
		DetailFromArgs: func(json.RawMessage) string { return "set_title" },
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			if err := ctx.Err(); err != nil {
				return tooldef.Result{}, err
			}
			if !utf8.Valid(input) {
				return tooldef.Result{}, errors.New("session args: invalid UTF-8")
			}
			var in struct {
				Action string `json:"action"`
				Title  string `json:"title"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("session args: %w", err)
			}
			if in.Action != "set_title" {
				return tooldef.Result{}, errors.New("session: use action=set_title")
			}
			if setTitle == nil {
				return tooldef.Result{}, errors.New("session naming unavailable")
			}
			title, err := setTitle(in.Title)
			if err != nil {
				return tooldef.Result{}, fmt.Errorf("session title: %w", err)
			}
			body := "Title: " + title
			return tooldef.Result{Content: body, Detail: title, Output: body}, nil
		},
	}
}
