package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulseaiclub/phi/internal/tools/tooldef"

	"github.com/pulseaiclub/phi/internal/llm"
)

var writeDescription = `Write content to a file. Creates the file if it does not exist; overwrites if it does. Creates parent directories. Use edit for surgical changes to an existing file.`

// WriteTool returns the write tool definition + handler.
func WriteTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "write",
			Description: writeDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "File path to write (created or overwritten). Example: src/new.go",
					},
					"content": llm.Object{
						"type":        "string",
						"description": "Content to write to the file.",
					},
				},
				Required: []string{"path", "content"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in writeInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Path)
		},
		Run: runWrite,
	}
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func runWrite(_ context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse write arguments: %w", err)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return tooldef.Result{}, errors.New("path is required")
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return tooldef.Result{}, err
		}
		path = filepath.Join(cwd, path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to create parent directories: %w", err)
	}

	//nolint:gosec // G306: source files should stay world-readable
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to write file %s: %w", path, err)
	}

	detail := fmt.Sprintf("wrote %d bytes to %s", len(in.Content), path)
	return tooldef.Result{Content: detail, Detail: path, Output: detail}, nil
}
