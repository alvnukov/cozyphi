package bashtool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
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
}

// ExecShell runs command via bash -c, streaming combined stdout+stderr.
func ExecShell(ctx context.Context, command string, opts ShellExecOptions) (ShellExecResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return ShellExecResult{}, fmt.Errorf("empty command")
	}

	cmd, err := buildShellCommand(ctx, command)
	if err != nil {
		return ShellExecResult{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ShellExecResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ShellExecResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return ShellExecResult{}, err
	}

	var (
		mu  sync.Mutex
		buf bytes.Buffer
		wg  sync.WaitGroup
	)
	stream := func(r io.Reader) {
		defer wg.Done()
		chunk := make([]byte, 4096)
		for {
			n, readErr := r.Read(chunk)
			if n > 0 {
				s := string(chunk[:n])
				mu.Lock()
				buf.WriteString(s)
				mu.Unlock()
				if opts.OnChunk != nil {
					opts.OnChunk(s)
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	wg.Add(2)
	go stream(stdout)
	go stream(stderr)

	waitErr := cmd.Wait()
	wg.Wait()

	out := FormatBashOutput(buf.String())

	res := ShellExecResult{Output: out}
	if errors.Is(ctx.Err(), context.Canceled) {
		res.Canceled = true
		return res, nil
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		return res, waitErr
	}
	return res, nil
}
