package readtool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/util"
)

const (
	readDefaultMaxLines = 1000
	readDefaultMaxBytes = 50 * 1024
)

var readDescription = fmt.Sprintf(`Read a file and return its contents.

Pass the file path; use offset (1-based) and limit to paginate. Output is capped
at %d lines and %d KiB per call.`, readDefaultMaxLines, readDefaultMaxBytes/1024)

// ReadTool returns the read tool definition + handler.
func ReadTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "read",
			Description: readDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "Path to an existing file. Example: src/main.go",
					},
					"offset": llm.Object{
						"type":        "integer",
						"description": "First line to return, 1-based. Example: 11",
					},
					"limit": llm.Object{
						"type":        "integer",
						"description": fmt.Sprintf("Maximum lines to return; capped at %d.", readDefaultMaxLines),
					},
				},
				Required: []string{"path"},
			},
			Readable: true,
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in readInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Path)
		},
		Run: runRead,
	}
}

type readInput struct {
	Path   string `json:"path"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func runRead(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse read arguments: %w", err)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return tooldef.Result{}, fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return tooldef.Result{}, err
		}
		path = filepath.Join(cwd, path)
	}

	f, err := os.Open(path)
	if err != nil {
		return tooldef.Result{}, err
	}
	defer f.Close()

	startLine := in.Offset
	if startLine < 1 {
		startLine = 1
	}
	limit := in.Limit
	if limit <= 0 || limit > readDefaultMaxLines {
		limit = readDefaultMaxLines
	}

	var (
		b         strings.Builder
		lineNo    int
		collected int
		bytesN    int
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return tooldef.Result{}, ctx.Err()
		default:
		}
		lineNo++
		if lineNo < startLine {
			continue
		}
		line := sc.Text()
		if bytesN+len(line)+1 > readDefaultMaxBytes {
			b.WriteString(fmt.Sprintf("\n... truncated at %d bytes. Next offset: %d\n", readDefaultMaxBytes, lineNo))
			break
		}
		// Use hashline format: LINE#HASH|content
		hash := util.ComputeLineHash(line)
		fmt.Fprintf(&b, "%d#%s|%s\n", lineNo, hash, line)
		bytesN += len(line) + 1
		collected++
		if collected >= limit {
			if sc.Scan() {
				b.WriteString(fmt.Sprintf("... truncated at %d lines. Next offset: %d\n", limit, lineNo+1))
			}
			break
		}
	}
	if err := sc.Err(); err != nil {
		return tooldef.Result{}, err
	}
	out := b.String()
	if out == "" {
		out = "(empty file)"
	}
	return tooldef.Result{Content: out, Detail: path, Output: out}, nil
}
