package readtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

const (
	readDefaultMaxLines = 1000
	readDefaultMaxBytes = 50 * 1024
	// Cap whole-file reads used for @file tags; larger files must be handled outside edit.
	readMaxHashBytes = 8 << 20 // 8 MiB
)

var readDescription = fmt.Sprintf(`Read a file with useful line numbers.

By default, mode:"view" returns N|content with no edit hashes or @file header.
Use mode:"edit" only when preparing an edit; it returns an @file path#TAG header
and N#HASH|content anchors and authorizes exactly those returned anchors for one edit attempt.
Use offset (1-based) and limit to paginate. Output is capped at %d lines and %d KiB per call.`,
	readDefaultMaxLines, readDefaultMaxBytes/1024)

// ReadTool returns the read tool definition + handler. An optional ledger lets
// a session registry share editable-read authorization with edit.
func ReadTool(ledgers ...*editledger.Ledger) tooldef.Tool {
	var ledger *editledger.Ledger
	if len(ledgers) > 0 {
		ledger = ledgers[0]
	}
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
					"mode": llm.Object{
						"type":        "string",
						"enum":        []string{"view", "edit"},
						"description": `Output mode: "view" (default) for plain numbered lines, or "edit" for authorized hashline anchors.`,
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
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			return runReadWithLedger(ctx, input, ledger)
		},
	}
}

type readInput struct {
	Path   string `json:"path"`
	Mode   string `json:"mode,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func runRead(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	return runReadWithLedger(ctx, input, nil)
}

func runReadWithLedger(ctx context.Context, input json.RawMessage, ledger *editledger.Ledger) (tooldef.Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse read arguments: %w", err)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return tooldef.Result{}, errors.New("path is required")
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "view"
	}
	if mode != "view" && mode != "edit" {
		return tooldef.Result{}, fmt.Errorf(`invalid read mode %q: expected "view" or "edit"`, in.Mode)
	}
	path, err := tooldef.ResolveToCwd(ctx, path)
	if err != nil {
		return tooldef.Result{}, err
	}

	if mode == "edit" {
		st, err := os.Stat(path)
		if err != nil {
			return tooldef.Result{}, err
		}
		if st.Size() > readMaxHashBytes {
			return tooldef.Result{}, fmt.Errorf(
				"file %s is %d bytes; refuse to hash files larger than %d bytes for edit anchors",
				path, st.Size(), readMaxHashBytes,
			)
		}
	}

	select {
	case <-ctx.Done():
		return tooldef.Result{}, ctx.Err()
	default:
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return tooldef.Result{}, err
	}
	text := util.NormalizeLF(string(raw))
	tag := ""
	if mode == "edit" {
		tag = util.ComputeFileHash(text)
	}
	display := tooldef.RelToCwd(ctx, path)
	startLine := in.Offset
	startLine = max(startLine, 1)
	limit := in.Limit
	if limit <= 0 || limit > readDefaultMaxLines {
		limit = readDefaultMaxLines
	}

	lines := strings.Split(text, "\n")
	// Trailing empty split from final newline is fine for line numbering.
	if text == "" {
		out := "(empty file)"
		if mode == "edit" {
			out = util.FormatFileHeader(display, tag) + "\n" + out
			ledger.Authorize(path, tag, nil)
		}
		return tooldef.Result{Content: out, Detail: display, Output: out}, nil
	}

	var (
		b         strings.Builder
		anchors   []string
		collected int
		bytesN    int
	)
	if mode == "edit" {
		b.WriteString(util.FormatFileHeader(display, tag))
		b.WriteByte('\n')
	}
	for lineNo := startLine; lineNo <= len(lines); lineNo++ {
		select {
		case <-ctx.Done():
			return tooldef.Result{}, ctx.Err()
		default:
		}
		line := lines[lineNo-1]
		if bytesN+len(line)+1 > readDefaultMaxBytes {
			fmt.Fprintf(&b, "\n... truncated at %d bytes. Next offset: %d\n", readDefaultMaxBytes, lineNo)
			break
		}
		if mode == "edit" {
			hash := util.ComputeLineHash(line)
			anchor := fmt.Sprintf("%d#%s", lineNo, hash)
			anchors = append(anchors, anchor)
			fmt.Fprintf(&b, "%s|%s\n", anchor, line)
		} else {
			fmt.Fprintf(&b, "%d|%s\n", lineNo, line)
		}
		bytesN += len(line) + 1
		collected++
		if collected >= limit {
			if lineNo < len(lines) {
				fmt.Fprintf(&b, "... truncated at %d lines. Next offset: %d\n", limit, lineNo+1)
			}
			break
		}
	}

	out := b.String()
	if mode == "edit" {
		ledger.Authorize(path, tag, anchors)
	}
	return tooldef.Result{Content: out, Detail: display, Output: out}, nil
}
