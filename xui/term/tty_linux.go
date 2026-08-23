//go:build linux || android

package term

import "syscall"

const (
	ioctlGetWinsize = syscall.TIOCGWINSZ
	ioctlGetTermios = 0x5401 // TCGETS
	ioctlSetTermios = 0x5402 // TCSETS
)
