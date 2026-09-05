//go:build unix && !solaris && !illumos && !aix

package session

import (
	"errors"
	"os"
	"syscall"
)

// tryOwnershipLock takes the exclusive owner lock without blocking.
func tryOwnershipLock(f *os.File) error {
	return tryFlock(f, syscall.LOCK_EX|syscall.LOCK_NB)
}

// tryProbeLock takes a shared lock: it conflicts with an owner but never with
// another probe, and it never makes a concurrent acquirer look busy for long.
func tryProbeLock(f *os.File) error {
	return tryFlock(f, syscall.LOCK_SH|syscall.LOCK_NB)
}

func tryFlock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrBusy
		}
		return err
	}
}
