package readtool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// A view of a file this size is streamed a page at a time instead.
	readMaxHashBytes = 8 << 20 // 8 MiB
	// Buffer one windowed read uses; long lines are consumed through it in slices.
	readStreamBufBytes = 64 << 10
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

	st, err := os.Stat(path)
	if err != nil {
		return tooldef.Result{}, err
	}
	if mode == "edit" && st.Size() > readMaxHashBytes {
		return tooldef.Result{}, fmt.Errorf(
			"file %s is %d bytes; refuse to hash files larger than %d bytes for edit anchors",
			path, st.Size(), readMaxHashBytes,
		)
	}

	select {
	case <-ctx.Done():
		return tooldef.Result{}, ctx.Err()
	default:
	}

	display := tooldef.RelToCwd(ctx, path)
	startLine := max(in.Offset, 1)
	limit := in.Limit
	if limit <= 0 || limit > readDefaultMaxLines {
		limit = readDefaultMaxLines
	}

	// Only a view can reach this size (edit refused it above). The answer is
	// one page either way, so the file is windowed off the disk instead of
	// being loaded whole with an index of every line. Lone-CR line breaks are
	// not split on this path; \r\n still is.
	if st.Size() > readMaxHashBytes {
		out, err := readViewWindow(ctx, path, startLine, limit)
		if err != nil {
			return tooldef.Result{}, err
		}
		return tooldef.Result{Content: out, Detail: display, Output: out}, nil
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

// readViewWindow renders the requested page of a file too large to hold in
// memory, in the same N|content form the in-memory path emits. It reads
// forward once and keeps at most one page, so cost is bounded by the offset,
// not by the file.
func readViewWindow(ctx context.Context, path string, startLine, limit int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, readStreamBufBytes)
	var (
		b         strings.Builder
		lineNo    int
		collected int
		bytesN    int
	)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		line, eof, err := readLineBounded(reader, readDefaultMaxBytes)
		if err != nil {
			return "", err
		}
		lineNo++
		if lineNo >= startLine {
			if bytesN+len(line)+1 > readDefaultMaxBytes {
				fmt.Fprintf(&b, "\n... truncated at %d bytes. Next offset: %d\n", readDefaultMaxBytes, lineNo)
				return b.String(), nil
			}
			fmt.Fprintf(&b, "%d|%s\n", lineNo, line)
			bytesN += len(line) + 1
			collected++
			if collected >= limit {
				if !eof {
					fmt.Fprintf(&b, "... truncated at %d lines. Next offset: %d\n", limit, lineNo+1)
				}
				return b.String(), nil
			}
		}
		if eof {
			return b.String(), nil
		}
	}
}

// readLineBounded reads one line, keeping at most maxBytes of it and
// discarding the rest: a minified bundle on one line must not be allocated in
// full just to be truncated by the page cap. eof reports that this was the
// last line in the file.
func readLineBounded(reader *bufio.Reader, maxBytes int) (line string, eof bool, err error) {
	var kept []byte
	for {
		chunk, readErr := reader.ReadSlice('\n')
		if room := maxBytes - len(kept); room > 0 {
			kept = append(kept, chunk[:min(room, len(chunk))]...)
		}
		switch {
		case errors.Is(readErr, bufio.ErrBufferFull):
			continue
		case errors.Is(readErr, io.EOF):
			return trimEOL(kept), true, nil
		case readErr != nil:
			return "", false, readErr
		}
		return trimEOL(kept), false, nil
	}
}

// trimEOL drops the line terminator the reader kept, so a CRLF file reads the
// same way NormalizeLF renders it in memory.
func trimEOL(line []byte) string {
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	return string(line)
}
