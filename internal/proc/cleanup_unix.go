//go:build !windows

package proc

import (
	"errors"
	"syscall"
)

// Unlike cancellation this must not fall back to a reaped leader's reused PID.
func cleanupProcessGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
