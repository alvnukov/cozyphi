package agent

import (
	"context"
	"errors"

	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// mutationGuard returns the re-check a mutating tool runs against the path it
// is about to change. The gate judged that path by its physical target when
// the call was approved; between the verdict and the rename a local process
// can swap a directory for a symlink, and the mutation module has no way to
// tell the redirected path from the approved one. Asking the gate again with
// the same path closes that window: a destination that now resolves outside
// the workspace, or into a sensitive path, is refused.
//
// consented names the path an Ask cleared for this one call — that is the
// user's consent, not the policy, so the re-check accepts the same Ask on
// that exact path and nothing else. Any other destination, including a
// redirected ancestor or a swapped leaf, still fails closed.
//
// A session without a gate returns nil and the write proceeds as before.
func (e *Executor) mutationGuard(toolName, consented string) tools.MutationGuard {
	if e.gate == nil {
		return nil
	}
	action := permission.ActionWrite
	if toolName == "edit" {
		action = permission.ActionEdit
	}
	return func(ctx context.Context, path string) error {
		req := permission.Request{Action: action, Tool: toolName, Paths: []string{path}}
		dec, reason := e.gate.Check(ctx, req)
		if dec == permission.Allow {
			return nil
		}
		if dec == permission.Ask && consented != "" {
			if want, errWant := permission.AbsClean(consented); errWant == nil {
				if got, errGot := permission.AbsClean(path); errGot == nil && got == want {
					return nil
				}
			}
		}
		if reason == "" {
			reason = "the permission gate no longer allows this path"
		}
		return errors.New(reason)
	}
}
