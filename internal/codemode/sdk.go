package codemode

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TypeScript declarations mirror each bound JSON schema. Tools whose output
// schema is unknown retain dynamic results until callers validate their data.
func sdkDeclarations(defs []llm.ToolDefinition) (string, error) {
	var text strings.Builder
	text.WriteString("type ToolArgs = Record<string, unknown>;\ndeclare const tools: {\n")
	text.WriteString(
		" [name: string]: (...args: any[]) => any;\n list(): string[];\n describe(name: string | {names:string[]}): unknown;\n",
	)
	for _, def := range defs {
		if def.Name == "call" || def.Name == "list" || def.Name == "describe" {
			continue
		}
		args := tsSchema{Type: "object"}
		if def.Params != nil {
			raw, err := json.Marshal(def.Params)
			if err != nil {
				return "", err
			}
			if err = json.Unmarshal(raw, &args); err != nil {
				return "", fmt.Errorf("tool schema %s: %w", def.Name, err)
			}
		}
		quoted, _ := json.Marshal(def.Name)
		opt := ""
		if len(args.Required) == 0 {
			opt = "?"
		}
		typ := args.declaration()
		fmt.Fprintf(&text, " %s(args%s: %s): Promise<any>;\n", quoted, opt, typ)
	}
	text.WriteString(" call(name: string, args?: ToolArgs): Promise<any>;\n};\n")
	return text.String(), nil
}

type tsSchema struct {
	Type       any
	Properties map[string]tsSchema
	Items      *tsSchema
	Enum       []any
	Required   []string
	AnyOf      []tsSchema
	OneOf      []tsSchema
}

func (s tsSchema) declaration() string {
	variants := append([]tsSchema(nil), s.AnyOf...)
	variants = append(variants, s.OneOf...)
	if len(variants) > 0 {
		parts := make([]string, 0, len(variants))
		for _, v := range variants {
			parts = append(parts, v.declaration())
		}
		return strings.Join(parts, " | ")
	}
	if len(s.Enum) > 0 {
		vals := make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			switch v.(type) {
			case nil, string, float64, bool:
				b, _ := json.Marshal(v)
				vals = append(vals, string(b))
			}
		}
		if len(vals) > 0 {
			return strings.Join(vals, " | ")
		}
	}
	if union, ok := s.Type.([]any); ok {
		parts := []string{}
		for _, t := range union {
			v := s
			v.Type = t
			parts = append(parts, v.declaration())
		}
		if len(parts) > 0 {
			return strings.Join(parts, " | ")
		}
	}
	kind, _ := s.Type.(string)
	switch kind {
	case "string", "boolean", "null":
		return kind
	case "integer", "number":
		return "number"
	case "array":
		if s.Items == nil {
			return "unknown[]"
		}
		return "Array<" + s.Items.declaration() + ">"
	case "object":
		if len(s.Properties) == 0 {
			return "Record<string, unknown>"
		}
		req := map[string]bool{}
		for _, name := range s.Required {
			req[name] = true
		}
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]string, 0, len(names))
		for _, name := range names {
			opt := "?"
			if req[name] {
				opt = ""
			}
			key, _ := json.Marshal(name)
			fields = append(fields, string(key)+opt+": "+s.Properties[name].declaration())
		}
		return "{ " + strings.Join(fields, "; ") + " }"
	default:
		return "unknown"
	}
}
