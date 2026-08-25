//go:build windows

package proc

import (
	"io"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const waitDelay = 3 * time.Second

// processGroupAttr hides the console window of spawned processes. Windows has
// no POSIX signals; trees are killed via taskkill.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

// killProcessTree terminates pid and its descendants via taskkill /T.
// Errors are ignored (best-effort): the process may already be gone, and
// Cmd.WaitDelay hard-kills the leader if the tree survives.
func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	kill.Stdout = io.Discard
	kill.Stderr = io.Discard
	_ = kill.Run()
	return nil
}
