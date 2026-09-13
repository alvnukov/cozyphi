//go:build !windows

package proc_test

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
)

func TestManagedGroupCleanupDoesNotChangeOrdinaryRun(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(fmt.Sprint(managed), func(t *testing.T) {
			result, err := proc.Run(t.Context(), proc.Spec{
				Argv: []string{"/bin/sh", "-c", `sleep 30 >/dev/null 2>&1 & printf '%s' "$!"`}, CleanupGroup: managed,
			}, proc.Limit{})
			require.NoError(t, err)
			pid, err := strconv.Atoi(strings.TrimSpace(result.Output))
			require.NoError(t, err)
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if !managed {
				require.NoError(
					t,
					syscall.Kill(pid, 0),
					"ordinary proc callers retain their established normal-exit semantics",
				)
				return
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				if err := syscall.Kill(pid, 0); err != nil {
					assert.ErrorIs(t, err, syscall.ESRCH)
					return
				}
				select {
				case <-ticker.C:
				case <-deadline.C:
					t.Fatal("managed descendant survived shell completion")
				}
			}
		})
	}
}
