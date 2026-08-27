package bashtool

import (
	"context"
	"errors"
	"strings"

	"github.com/alvnukov/cozyphi/internal/proc"
)

// ShellExecResult is the outcome of a streaming shell run.
type ShellExecResult struct {
	Output   string
	ExitCode int
	Canceled bool
}

// ShellExecOptions configures interactive / streaming shell execution.
type ShellExecOptions struct {
	OnChunk func(chunk string)
	// CollectLimit bounds the output retained for Result.Output; <= 0 selects
	// BashMaxCollectBytes. A caller that consumes OnChunk as it arrives and
	// never reads Output asks for a small budget instead of the default 8 MB.
	CollectLimit int
}

// ExecShell runs command via bash -c, streaming combined stdout+stderr.
// Collection is bounded independently of streaming callbacks, whose consumers
// are responsible for applying their own retention limits.
func ExecShell(ctx context.Context, command string, opts ShellExecOptions) (ShellExecResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return ShellExecResult{}, errors.New("empty command")
	}

	spec, err := buildShellSpec(ctx, command)
	if err != nil {
		return ShellExecResult{}, err
	}
	spec.Stream = opts.OnChunk

	collect := opts.CollectLimit
	if collect <= 0 {
		collect = BashMaxCollectBytes
	}
	res, err := proc.Run(ctx, spec, proc.Limit{Bytes: collect})
	if err != nil {
		return ShellExecResult{}, err
	}
	return ShellExecResult{
		Output:   formatBashOutput(res.Output, res.Truncated),
		ExitCode: res.ExitCode,
		Canceled: res.Canceled,
	}, nil
}
