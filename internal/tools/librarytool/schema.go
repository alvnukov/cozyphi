package librarytool

import "github.com/alvnukov/cozyphi/internal/llm"

func params(actions []string) *llm.FunctionParameters {
	text := func(desc string) llm.Object { return llm.Object{"type": "string", "description": desc} }
	edits := llm.Object{
		"type": "array",
		"items": llm.Object{
			"type": "object", "required": []string{"from", "to"},
			"properties": llm.Object{
				"from": text("Start LINE#HASH anchor."), "to": text("End LINE#HASH anchor, inclusive."),
				"content": text("Replacement text; omit to delete these lines."),
			},
		},
	}
	return &llm.FunctionParameters{
		Type: "object", Required: []string{"action"},
		Properties: llm.Object{
			"action":  llm.Object{"type": "string", "enum": actions},
			"name":    text("Logical package name."),
			"version": text("Exact semantic release version, e.g. 1.2.0."),
			"file":    llm.Object{"type": "string", "enum": []string{"index.ts", "index.test.ts", "package.json"}},
			"query":   text("Task, export, or package to find."),
			"versions": llm.Object{
				"type":                 "object",
				"additionalProperties": llm.Object{"type": "string"},
				"description":          "For pin, exact versions keyed by package to update atomically.",
			},
			"revision": text("Exact current draft digest from open, read, edit, diff or test. Required for publish."),
			"content":  text("New file contents, for create_file only."),
			"hash":     text("Draft file TAG from its editable read, for edit."),
			"edits":    edits,
			"offset":   llm.Object{"type": "integer", "description": "First source line or package, 1-based."},
			"limit":    llm.Object{"type": "integer", "description": "Maximum lines, exports or packages to return."},
		},
	}
}
