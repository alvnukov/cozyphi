package writetool

import (
	"context"

	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// mutationGuard adapts the session's permission re-check to the mutation
// module. The gate judged this path when the call was approved; the guard asks
// it again while the write is in flight, so a directory swapped for a symlink
// in between cannot redirect the file. Sessions without a gate carry no guard
// and the write proceeds as before.
func mutationGuard(ctx context.Context) func(path string) error {
	return func(path string) error {
		return tooldef.GuardMutation(ctx, path)
	}
}
