package bashtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/llm"
)

const (
	bashDefaultTimeout = 300
)

var bashDescription = `Run a shell command and return combined stdout/stderr.

Use for build, test, git, and OS tasks that read/ls/find/grep/edit/write cannot
do. Do not use for cat, head, tail, ls(1), find(1), grep, or rg — those have dedicated
tools. Large output is truncated to its tail with the full output written to a
temp file. Lines that mark a failure (test FAIL, compiler file:line:col, panic,
traceback) are listed up front with their line numbers, and a zero exit that
hides such lines is flagged: a pipeline reports only its last stage.`

// BashTool returns the bash tool definition + handler.
func BashTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "bash",
			Description: bashDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"command": llm.Object{
						"type":        "string",
						"description": "Shell command to run. Example: go test ./...",
					},
					"timeout": llm.Object{
						"type":        "integer",
						"description": "Timeout in seconds, 1-3600. Example: 120 (default: 300).",
					},
				},
				Required: []string{"command"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in bashInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Command)
		},
		Run: runBash,
	}
}

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func runBash(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse bash arguments: %w", err)
	}
	cmd := strings.TrimSpace(in.Command)
	if cmd == "" {
		return tooldef.Result{}, errors.New("empty command")
	}

	timeout := in.Timeout
	if timeout <= 0 {
		timeout = bashDefaultTimeout
	}
	if timeout > 3600 {
		timeout = 3600
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	spec, err := buildShellSpec(ctx, cmd)
	if err != nil {
		return tooldef.Result{}, err
	}
	res, err := proc.Run(ctx, spec, proc.Limit{Bytes: BashMaxCollectBytes})
	if err != nil {
		return tooldef.Result{}, err
	}

	content, display := bashReport(res.Output, res.Truncated, res.ExitCode, res.Canceled)
	return tooldef.Result{Content: content, Detail: cmd, Output: display}, nil
}

// bashReport renders one run for its two readers. display is the tail the
// TUI shows, with the exit or cancellation footer. content is what the model
// reads: the same display, preceded by the failure block when the output is
// long enough to hide its failures. Both carry the masked-failure note — a
// zero exit that the output contradicts is news to the user too.
func bashReport(output string, collectionTruncated bool, exitCode int, canceled bool) (content, display string) {
	display = formatBashOutput(output, collectionTruncated)
	if strings.TrimSpace(display) == "" {
		display = "(no output)"
	}
	scan := scanFailures(output)
	switch {
	case canceled:
		display += "\n(command canceled or timed out)"
	case exitCode != 0:
		display += fmt.Sprintf("\n(exit error: exit status %d)", exitCode)
	case scan.Markers > 0:
		display += "\n" + maskedFailureNote(scan)
	}
	content = display
	if block := failureBlock(scan, collectionTruncated); block != "" {
		content = block + "\n\n" + display
	}
	return content, display
}
