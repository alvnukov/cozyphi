//go:build windows

package session

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func tryOwnershipLock(f *os.File) error {
	const (
		lockfileFailImmediately = 0x00000001
		lockfileExclusiveLock   = 0x00000002
		errorLockViolation      = syscall.Errno(33)
	)
	// Lock one byte beyond EOF: the stable sidecar intentionally stays empty.
	// No OVERLAPPED I/O is pending because FAIL_IMMEDIATELY is set.
	var overlapped syscall.Overlapped
	ok, _, err := lockFileEx.Call(
		f.Fd(), lockfileFailImmediately|lockfileExclusiveLock, 0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	runtime.KeepAlive(f)
	if ok != 0 {
		return nil
	}
	if err == errorLockViolation {
		return ErrBusy
	}
	return err
}
