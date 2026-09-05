//go:build windows

package session

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

// tryOwnershipLock takes the exclusive owner lock without blocking.
func tryOwnershipLock(f *os.File) error {
	return tryLockFileEx(f, lockfileFailImmediately|lockfileExclusiveLock)
}

// tryProbeLock takes a shared lock: it conflicts with an owner but never with
// another probe, and it never makes a concurrent acquirer look busy for long.
func tryProbeLock(f *os.File) error {
	return tryLockFileEx(f, lockfileFailImmediately)
}

func tryLockFileEx(f *os.File, flags uintptr) error {
	// Lock one byte beyond EOF: the stable sidecar intentionally stays empty.
	// No OVERLAPPED I/O is pending because FAIL_IMMEDIATELY is set.
	var overlapped syscall.Overlapped
	ok, _, err := lockFileEx.Call(f.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(f)
	if ok != 0 {
		return nil
	}
	if err == errorLockViolation {
		return ErrBusy
	}
	return err
}
