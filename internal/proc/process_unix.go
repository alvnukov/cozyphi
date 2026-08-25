//go:build !windows

package proc

import (
	"syscall"
	"time"
)

const waitDelay = 3 * time.Second

// processGroupAttr puts each child in its own process group so the whole tree
// can be killed with a single negative-pid signal.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree kills pid and all descendants (the process group), falling
// back to killing the leader only if the group is already gone.
func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}
