// Package lsptool adapts the frozen lsp.QueryFunc into one model-facing tool.
// The model never sees commands, argv, env, roots, PIDs, start/stop controls,
// or raw LSP methods — only the compact op/file/position schema and bounded
// normalized output.
package lsptool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

var lspDescription = `Query the harness-managed Go language server (gopls). Read-only code
intelligence; prefer it over text search for structural questions — who calls
this, where is this defined, what implements this — a search matches text,
the server resolves names.

Operations:
- definition: where the symbol is declared.
- references: everywhere the symbol is used; include_declaration defaults
  to true.
- implementations: implementations of an interface or interface method, and
  the interfaces a concrete type satisfies.
- type_definition: declaration of the type of the expression at the target.
- hover: signature, type, and docs.
- symbols: outline of one file, a workspace-wide name search by query, or
  both — file plus query filters that file's outline.
- calls: incoming (callers) or outgoing (callees) call hierarchy; direction
  defaults to incoming.
- diagnostics: current diagnostics for one file after harness-managed sync;
  reports fresh, cached, unconfirmed, or pending provenance.
- languages: status of the harness-managed Go server (configured,
  installed, running, supported operations); takes no other fields.

Targeting (definition/references/implementations/type_definition/hover/calls):
give a symbol name, a file with 1-based line+character (from a read/grep
header), or any combination — a position picks between several declarations
of one name. file is optional with symbol: the name is then resolved
workspace-wide, and an ambiguous or unknown name answers with the candidate
declarations to choose from. The symbol only has to appear in the file, not
be declared there, and Container.Name is accepted. Location results carry the
source line as a snippet, so a follow-up read is usually unnecessary.
Results are bounded, workspace-relative, and never expose raw LSP payloads.`

// Tool binds the shared query function into the lsp tool. A nil query disables
// the capability entirely.
func Tool(query lsp.QueryFunc) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "lsp",
			Description: lspDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"op": llm.Object{
						"type": "string",
						"enum": []string{
							"languages",
							"definition",
							"references",
							"implementations",
							"type_definition",
							"hover",
							"symbols",
							"calls",
							"diagnostics",
						},
						"description": "Code-intelligence operation to run.",
					},
					"file": llm.Object{
						"type":        "string",
						"description": "Target file path (cwd-relative or absolute). Required for diagnostics and positions; optional with symbol, which then resolves workspace-wide.",
					},
					"symbol": llm.Object{
						"type":        "string",
						"description": "Symbol name target; Container.Name is accepted, and the name only has to appear in the file, not be declared there. Combine with line (and character) to pick between declarations.",
					},
					"line": llm.Object{
						"type":        "integer",
						"minimum":     1,
						"description": "1-based line in the target file.",
					},
					"character": llm.Object{
						"type":        "integer",
						"minimum":     1,
						"description": "1-based Unicode code-point column in the target file.",
					},
					"query": llm.Object{
						"type":        "string",
						"description": "Workspace symbol query (symbols op); with file, filters that file's outline.",
					},
					"direction": llm.Object{
						"type":        "string",
						"enum":        []string{"incoming", "outgoing"},
						"description": "Call hierarchy direction (calls op only); defaults to incoming.",
					},
					"include_declaration": llm.Object{
						"type":        "boolean",
						"description": "Include the declaration in references results; defaults true (references op only).",
					},
					"limit": llm.Object{
						"type":    "integer",
						"minimum": 1,
						"description": fmt.Sprintf(
							"Maximum results; default %d, hard max %d.",
							lsp.DefaultItemLimit,
							lsp.MaxItemLimit,
						),
					},
				},
				Required: []string{"op"},
			},
			Readable: true,
		},
		DetailFromArgs: detailFromArgs,
		Run:            run(query),
	}
}

type input struct {
	Op                 string  `json:"op"`
	File               *string `json:"file"`
	Symbol             *string `json:"symbol"`
	Line               *int    `json:"line"`
	Character          *int    `json:"character"`
	Query              *string `json:"query"`
	Direction          *string `json:"direction"`
	IncludeDeclaration *bool   `json:"include_declaration"`
	Limit              *int    `json:"limit"`
	// PlanStep is injected by the plan gate and consumed before this tool runs;
	// it is accepted here so strict decoding never rejects a gate-valid call.
	PlanStep tooldef.PlanStep `json:"plan_step"`
}

func run(query lsp.QueryFunc) tooldef.Handler {
	return func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
		in, err := parse(raw)
		if err != nil {
			return tooldef.Result{}, err
		}
		q, err := build(ctx, in)
		if err != nil {
			return tooldef.Result{}, err
		}
		res, err := query(ctx, q)
		if err != nil {
			return tooldef.Result{}, err
		}
		content := render(q.Op, res)
		return tooldef.Result{Content: content, Detail: detail(q), Output: content}, nil
	}
}

// parse decodes with unknown-field rejection before any process start.
func parse(raw json.RawMessage) (input, error) {
	var in input
	if err := tooldef.DecodeStrict(raw, &in); err != nil {
		return in, fmt.Errorf("lsp: invalid arguments: %w", err)
	}
	return in, nil
}

// build validates the frozen matrix and resolves file against the engine cwd.
func build(ctx context.Context, in input) (lsp.Query, error) {
	op := lsp.Operation(strings.TrimSpace(in.Op))
	q := lsp.Query{Op: op}
	if in.File != nil {
		path, err := tooldef.ResolveToCwd(ctx, strings.TrimSpace(*in.File))
		if err != nil {
			return q, err
		}
		q.File = path
	}
	if in.Symbol != nil {
		q.Symbol = strings.TrimSpace(*in.Symbol)
	}
	if in.Query != nil {
		q.Query = strings.TrimSpace(*in.Query)
	}
	if in.Line != nil {
		q.Line = *in.Line
	}
	if in.Character != nil {
		q.Character = *in.Character
	}
	if in.Direction != nil {
		q.Direction = lsp.Direction(strings.TrimSpace(*in.Direction))
	}
	if in.IncludeDeclaration != nil {
		q.IncludeDeclaration = *in.IncludeDeclaration
	} else if op == lsp.OpReferences {
		// The frozen default: omitting the flag includes the declaration.
		q.IncludeDeclaration = true
	}
	if in.Limit != nil {
		q.Limit = *in.Limit
		if q.Limit < 1 {
			return q, errors.New("lsp: limit must be at least 1")
		}
	}

	if in.IncludeDeclaration != nil && op != lsp.OpReferences {
		return q, errors.New("lsp: include_declaration applies only to references")
	}
	// The Manager re-validates the absolute path and matrix; this early pass
	// keeps irrelevant combinations from ever reaching a process. The
	// navigational matrix is tolerant on purpose: a symbol, a position, or
	// both are all valid targets — the model routinely sends everything it
	// knows, and refusing that only costs a retry round.
	switch op {
	case lsp.OpLanguages:
		if in.File != nil || in.Symbol != nil || in.Query != nil || in.Line != nil || in.Character != nil ||
			in.Direction != nil {
			return q, errors.New("lsp: languages takes no target fields")
		}
	case lsp.OpDefinition, lsp.OpReferences, lsp.OpImplementations, lsp.OpTypeDefinition, lsp.OpHover, lsp.OpCalls:
		if in.Line != nil && *in.Line < 1 || in.Character != nil && *in.Character < 1 {
			return q, fmt.Errorf("lsp: %s requires 1-based line and character", op)
		}
		if in.Character != nil && in.Line == nil {
			return q, fmt.Errorf("lsp: %s: character requires line", op)
		}
		if in.Line != nil && in.File == nil {
			return q, fmt.Errorf("lsp: %s: line requires file", op)
		}
		if q.Symbol == "" {
			if in.File == nil {
				return q, fmt.Errorf("lsp: %s requires symbol or file with line+character", op)
			}
			if in.Line == nil {
				return q, fmt.Errorf("lsp: %s requires symbol or line+character", op)
			}
			if in.Character == nil {
				return q, fmt.Errorf("lsp: %s with line alone needs character or symbol", op)
			}
		}
		if op == lsp.OpCalls {
			if in.Direction == nil {
				q.Direction = lsp.DirectionIncoming
			} else if q.Direction != lsp.DirectionIncoming && q.Direction != lsp.DirectionOutgoing {
				return q, errors.New("lsp: calls requires direction incoming|outgoing")
			}
		}
	case lsp.OpSymbols:
		if in.File == nil && in.Query == nil {
			return q, errors.New("lsp: symbols requires file or query")
		}
	case lsp.OpDiagnostics:
		if in.File == nil {
			return q, errors.New("lsp: diagnostics requires file")
		}
	default:
		return q, fmt.Errorf("lsp: unknown operation %q", in.Op)
	}
	return q, nil
}

func detail(q lsp.Query) string {
	parts := []string{string(q.Op)}
	if q.File != "" {
		parts = append(parts, q.File)
	} else if q.Query != "" {
		parts = append(parts, q.Query)
	} else if q.Symbol != "" {
		parts = append(parts, q.Symbol)
	}
	return strings.Join(parts, " ")
}

func detailFromArgs(raw json.RawMessage) string {
	var in input
	_ = json.Unmarshal(raw, &in)
	op := strings.TrimSpace(in.Op)
	if op == "" {
		op = "lsp"
	}
	out := op
	if in.File != nil && strings.TrimSpace(*in.File) != "" {
		out += " " + strings.TrimSpace(*in.File)
	} else if in.Query != nil && strings.TrimSpace(*in.Query) != "" {
		out += " " + strings.TrimSpace(*in.Query)
	} else if in.Symbol != nil && strings.TrimSpace(*in.Symbol) != "" {
		out += " " + strings.TrimSpace(*in.Symbol)
	}
	return out
}
