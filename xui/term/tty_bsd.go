//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package term

import "syscall"

const (
	ioctlGetWinsize = syscall.TIOCGWINSZ
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)
