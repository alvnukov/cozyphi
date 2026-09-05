//go:build linux || darwin

package session

import (
	"errors"
	"os"
	"syscall"
)

func tryOwnershipLock(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrBusy
		}
		return err
	}
}
