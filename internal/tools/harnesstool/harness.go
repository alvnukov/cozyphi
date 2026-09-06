// Package harnesstool exposes cozyphi's own configuration to the model as
// one read-only tool.
//
// It is registered only when the user started the process with
// --developer-mode. The tool observes; it never changes anything, and there
// is no write action to add later without a new decision.
package harnesstool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// Actions the tool accepts.
const (
	actionCatalog  = "catalog"
	actionSnapshot = "snapshot"
	actionExplain  = "explain"
)

// refusal is what every call gets when the process carries no diagnostics
// registry. It is the answer to a direct call made without the capability —
// including one that reaches here through a registration bug — and it says
// what would grant it rather than hinting at what it would have shown.
const refusal = "harness: this session was not started with --developer-mode, so it observes nothing. " +
	"There is no way to enable it from here; the user must restart cozyphi with the flag"

// description reaches the model on every turn of a developer-mode session,
// so it says what the tool does, what it cannot do, and what it will not
// show — a model that expects raw config here wastes turns discovering it
// is not on offer.
const description = `Read cozyphi's own configuration: what this process is running with, where each
value came from, and what it would take to change it.

Read-only. It observes the harness and changes nothing — there is no set, no
reload, no reset. Observing runs no command, opens no connection and touches
no file.

# Actions

- catalog: every category that exists, whether it is wired yet, and the field
  keys it declares. Start here — a category that is not implemented says so,
  and no other action will invent data for it.
- snapshot: what is in effect now. Without a category it is the overview —
  every category, each field reduced to the value acting right now. With a
  category it is that category alone, with all three layers and where each
  came from.
- explain: one field, with everything. Needs both category and key.

# What one field carries

Three layers, because they disagree more often than they look like they
should, and the disagreement is usually the answer: configured is what a
source asked for, loaded is what the owner took in, effective is what is
acting right now. Each layer names its own source. A field also says what it
would take for a change to land (immediate, next_turn, reload, new_session,
restart) and how far it reaches (process, session, workspace, turn, step).

An empty-looking answer is never ambiguous: unset, redacted, unavailable and
not_applicable are four different states, and false and 0 are real values
that always appear.

# What it will not show

Secrets are reported as presence and source, never as values, hashes or
suffixes. Raw config structs, environment variables, prompts, transcripts,
memory contents, logs, hook and watch commands, and MCP server arguments are
not exported at all. Paths, URLs and error text are sanitized before they
reach you, so a value here may read shorter than the original.

Answers are size-bounded. When one says truncated, narrow it: pick a
category, then a key. There is no paging.`

// Deps binds the tool to the process's diagnostics registry. A nil registry
// is the no-capability case and yields a tool that refuses safely rather
// than one that panics or half-answers.
type Deps struct {
	Registry *diag.Registry
}

// Tool returns the harness tool definition and handler.
func Tool(deps Deps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "harness",
			Description: description,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type": "string",
						"enum": []string{actionCatalog, actionSnapshot, actionExplain},
						"description": "catalog: what can be observed. snapshot: what is in effect now. " +
							"explain: one field in full.",
					},
					"category": llm.Object{
						"type": "string",
						"enum": categoryNames(),
						"description": "Which category to read. Omit on snapshot for the overview; " +
							"required for explain; not accepted for catalog.",
					},
					"key": llm.Object{
						"type":        "string",
						"description": "Which field to explain, as catalog names it (explain action only).",
					},
				},
				Required: []string{"action"},
			},
		},
		DetailFromArgs: detail,
		Run:            run(deps.Registry),
	}
}

// input is the tool's argument shape. Unknown fields are rejected by
// DecodeStrict, so plan_step is declared here to keep a gate-valid call
// decodable; the executor consumes it before this tool ever runs.
type input struct {
	Action   string           `json:"action"`
	Category string           `json:"category"`
	Key      string           `json:"key"`
	PlanStep tooldef.PlanStep `json:"plan_step"`
}

func run(registry *diag.Registry) tooldef.Handler {
	return func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
		if registry == nil {
			return tooldef.Result{}, errors.New(refusal)
		}
		in, err := parse(raw)
		if err != nil {
			return tooldef.Result{}, err
		}
		switch in.Action {
		case actionCatalog:
			return tooldef.Result{Content: registry.Catalog().JSON(), Detail: actionCatalog}, nil
		case actionSnapshot:
			return snapshot(ctx, registry, in.Category)
		default:
			return explain(ctx, registry, in.Category, in.Key)
		}
	}
}

func snapshot(ctx context.Context, registry *diag.Registry, category string) (tooldef.Result, error) {
	parsed, err := parseCategory(category)
	if err != nil {
		return tooldef.Result{}, err
	}
	snap, err := registry.Snapshot(ctx, parsed)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("harness: %w", err)
	}
	return tooldef.Result{Content: snap.JSON(), Detail: detailText(actionSnapshot, category, "")}, nil
}

func explain(ctx context.Context, registry *diag.Registry, category, key string) (tooldef.Result, error) {
	parsed, err := parseCategory(category)
	if err != nil {
		return tooldef.Result{}, err
	}
	explanation, err := registry.Explain(ctx, parsed, key)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("harness: %w", err)
	}
	return tooldef.Result{
		Content: explanation.JSON(),
		Detail:  detailText(actionExplain, category, key),
	}, nil
}

// parse validates the argument shape before anything is observed: a wrong
// action, an unknown category or a missing key costs an error the model can
// act on, not a plausible-looking answer to a question it did not ask.
func parse(raw json.RawMessage) (input, error) {
	var in input
	if err := tooldef.DecodeStrict(raw, &in); err != nil {
		return input{}, fmt.Errorf("harness: invalid arguments: %w", err)
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	in.Key = strings.TrimSpace(in.Key)

	switch in.Action {
	case "":
		return input{}, errors.New("harness: action is required (catalog, snapshot or explain)")
	case actionCatalog:
		if in.Category != "" || in.Key != "" {
			return input{}, errors.New(
				"harness: catalog takes no category or key; it lists every category. " +
					"Use action=snapshot with a category to read one")
		}
	case actionSnapshot:
		if in.Key != "" {
			return input{}, errors.New(
				"harness: snapshot takes no key; omit it for the whole category, " +
					"or use action=explain with category and key for one field")
		}
	case actionExplain:
		if in.Category == "" || in.Key == "" {
			return input{}, errors.New(
				"harness: explain needs both category and key; action=catalog lists the keys of each category")
		}
	default:
		return input{}, fmt.Errorf(
			"harness: unknown action %q (use catalog, snapshot or explain)", safeEcho(in.Action))
	}
	return in, nil
}

// parseCategory maps the model's string onto the catalog. An empty string is
// the overview and is only reachable from snapshot, which parse has already
// checked.
func parseCategory(name string) (diag.Category, error) {
	if name == "" {
		return "", nil
	}
	category, ok := diag.ParseCategory(name)
	if !ok {
		return "", fmt.Errorf(
			"harness: unknown category %q; known categories: %s",
			safeEcho(name), strings.Join(categoryNames(), ", "))
	}
	return category, nil
}

func categoryNames() []string {
	categories := diag.Categories()
	names := make([]string, len(categories))
	for i, category := range categories {
		names[i] = string(category)
	}
	return names
}

// safeEcho bounds a model-supplied string before it is quoted back into an
// error the user will read in the transcript.
func safeEcho(s string) string {
	const limit = 40
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}

// detail is the one line the UI shows before the tool runs; it is computed
// from the raw arguments, which at that point have not been validated.
func detail(raw json.RawMessage) string {
	var in input
	_ = json.Unmarshal(raw, &in)
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		action = actionSnapshot
	}
	return detailText(action, strings.TrimSpace(in.Category), strings.TrimSpace(in.Key))
}

func detailText(action, category, key string) string {
	parts := make([]string, 0, 3)
	parts = append(parts, safeEcho(action))
	if category != "" {
		parts = append(parts, safeEcho(category))
	}
	if key != "" {
		parts = append(parts, safeEcho(key))
	}
	return strings.Join(parts, " ")
}
