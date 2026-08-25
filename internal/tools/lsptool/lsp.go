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
intelligence; the model cannot start, stop, or configure the server.

Operations:
- definition: exact position (line+character, both 1-based) of a symbol.
- references, hover, calls, symbols, diagnostics, languages: reserved V1
  operations; exact-position definition is the first implemented op.

Use line+character from a read/grep header (1-based). Results are bounded,
workspace-relative, and never expose raw LSP payloads.`

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
							"hover",
							"symbols",
							"calls",
							"diagnostics",
						},
						"description": "Code-intelligence operation to run.",
					},
					"file": llm.Object{
						"type":        "string",
						"description": "Target file path (cwd-relative or absolute). Required for definition/references/hover/calls/diagnostics and file symbols.",
					},
					"symbol": llm.Object{
						"type":        "string",
						"description": "Symbol name target (alternative to line+character where supported).",
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
						"description": "Workspace symbol query (symbols op).",
					},
					"direction": llm.Object{
						"type":        "string",
						"enum":        []string{"incoming", "outgoing"},
						"description": "Call hierarchy direction (calls op only).",
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
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, fmt.Errorf("lsp: invalid arguments: %w", err)
	}
	if dec.More() {
		return in, errors.New("lsp: trailing JSON data")
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
	// keeps irrelevant combinations from ever reaching a process.
	switch op {
	case lsp.OpDefinition:
		if in.File == nil {
			return q, errors.New("lsp: definition requires file")
		}
		if in.Symbol != nil {
			return q, errors.New("lsp: definition by symbol is not implemented")
		}
		if in.Line == nil || in.Character == nil || *in.Line < 1 || *in.Character < 1 {
			return q, errors.New("lsp: definition requires 1-based line and character")
		}
	case lsp.OpLanguages:
		if in.File != nil || in.Symbol != nil || in.Query != nil || in.Line != nil || in.Character != nil ||
			in.Direction != nil {
			return q, errors.New("lsp: languages takes no target fields")
		}
	case lsp.OpReferences, lsp.OpHover, lsp.OpCalls:
		if in.File == nil {
			return q, fmt.Errorf("lsp: %s requires file", op)
		}
		hasSym := in.Symbol != nil
		hasPos := in.Line != nil || in.Character != nil
		if hasSym == hasPos {
			return q, fmt.Errorf("lsp: %s requires symbol or line+character, not both", op)
		}
		if hasPos && (in.Line == nil || in.Character == nil || *in.Line < 1 || *in.Character < 1) {
			return q, fmt.Errorf("lsp: %s requires 1-based line and character", op)
		}
		if op == lsp.OpCalls {
			if in.Direction == nil || (q.Direction != lsp.DirectionIncoming && q.Direction != lsp.DirectionOutgoing) {
				return q, errors.New("lsp: calls requires direction incoming|outgoing")
			}
		}
	case lsp.OpSymbols:
		if in.File == nil && in.Query == nil {
			return q, errors.New("lsp: symbols requires file or query")
		}
		if in.File != nil && in.Query != nil {
			return q, errors.New("lsp: symbols accepts file or query, not both")
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
