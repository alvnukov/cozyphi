package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

var writeDescription = `Write content to a file. Creates the file if it does not exist; overwrites the entire file if it does. Creates parent directories.`

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

func runWrite(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse write arguments: %w", err)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return tooldef.Result{}, errors.New("path is required")
	}
	path, err := tooldef.ResolveToCwd(ctx, path)
	if err != nil {
		return tooldef.Result{}, err
	}

	// Captured before the write so the transcript diff card shows what this
	// call actually changed; a file that does not exist yet diffs against
	// nothing and the whole write reads as additions. The read refuses a leaf
	// symlink, so a swapped link cannot leak foreign content into the diff.
	old := ""
	if data, readErr := atomicfile.ReadNoFollow(path); readErr == nil {
		old = util.NormalizeLF(string(data))
	}

	// The swap is staged and renamed into place by the shared mutation module:
	// a symlink swapped in after the permission check is refused (or replaced
	// by the rename, never written through), and a torn write cannot happen.
	if err := atomicfile.Write(path, 0o644, []byte(in.Content)); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to write file %s: %w", path, err)
	}

	display := tooldef.RelToCwd(ctx, path)
	detail := fmt.Sprintf("wrote %d bytes to %s", len(in.Content), display)
	diff := util.GenerateFileDiff(path, old, util.NormalizeLF(in.Content), 3)
	return tooldef.Result{Content: detail, Detail: display, Output: diff}, nil
}
