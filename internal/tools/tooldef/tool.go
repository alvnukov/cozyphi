package tooldef

import (
	"context"
	"encoding/json"

	"github.com/pulseaiclub/phi/internal/llm"
)

// Result is what a tool returns to the model / UI.
type Result struct {
	// Content is sent back to the model as the tool message body.
	Content string
	// Detail is a short one-line summary for the TUI tool row.
	Detail string
	// Output is the display body for the TUI (may equal Content).
	Output string
}

// Handler runs a tool given raw JSON arguments.
type Handler func(ctx context.Context, input json.RawMessage) (Result, error)

// Tool is a schema + implementation pair.
type Tool struct {
	Definition llm.ToolDefinition
	Run        Handler
	// DetailFromArgs extracts a one-line detail for the UI before execution.
	DetailFromArgs func(input json.RawMessage) string
}

// Definitions extracts LLM schemas from tools.
func Definitions(tools []Tool) []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, len(tools))
	for i, t := range tools {
		out[i] = t.Definition
	}
	return out
}

// Registry maps tool name → Tool.
type Registry map[string]Tool

// NewRegistry indexes tools by name.
func NewRegistry(tools []Tool) Registry {
	m := make(Registry, len(tools))
	for _, t := range tools {
		m[t.Definition.Name] = t
	}
	return m
}
