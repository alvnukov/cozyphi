package proc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Cleanup is part of the managed process contract. Its failure must not be
// swallowed by the ordinary exit/cancellation paths used by both stream modes.
func classifyRunResult(res Result, runErr, cleanupErr, contextErr error) (Result, error) {
	if cleanupErr != nil {
		return res, fmt.Errorf("proc: clean managed process group: %w", errors.Join(runErr, cleanupErr))
	}
	if runErr == nil {
		return res, nil
	}
	if errors.Is(contextErr, context.Canceled) {
		res.Canceled = true
		return res, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("proc: run: %w", runErr)
}
