//go:build unix

package term

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func openTTY() (TTY, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		if !isTerminal(int(os.Stdin.Fd())) {
			return nil, fmt.Errorf("xui: no tty: %w", err)
		}
		return &unixTTY{file: os.Stdin, out: os.Stdout, owns: false}, nil
	}
	return &unixTTY{file: f, out: f, owns: true}, nil
}

type unixTTY struct {
	file   *os.File
	out    *os.File
	owns   bool
	orig   syscall.Termios
	rawSet bool
}

func (t *unixTTY) Read(p []byte) (int, error)  { return t.file.Read(p) }
func (t *unixTTY) Write(p []byte) (int, error) { return t.out.Write(p) }
func (t *unixTTY) Fd() uintptr                 { return t.file.Fd() }

func (t *unixTTY) Size() (cols, rows int, err error) {
	var ws winsize
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		t.file.Fd(),
		ioctlGetWinsize,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 80, 24, errno
	}
	if ws.Col == 0 || ws.Row == 0 {
		return 80, 24, nil
	}
	return int(ws.Col), int(ws.Row), nil
}

func (t *unixTTY) MakeRaw() error {
	fd := int(t.file.Fd())
	var term syscall.Termios
	if err := ioctlTermios(fd, ioctlGetTermios, &term); err != nil {
		return err
	}
	t.orig = term
	term.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	term.Oflag &^= syscall.OPOST
	term.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	term.Cflag &^= syscall.CSIZE | syscall.PARENB
	term.Cflag |= syscall.CS8
	term.Cc[syscall.VMIN] = 1
	term.Cc[syscall.VTIME] = 0
	if err := ioctlTermios(fd, ioctlSetTermios, &term); err != nil {
		return err
	}
	t.rawSet = true
	return nil
}

func (t *unixTTY) Restore() error {
	if !t.rawSet {
		return nil
	}
	return ioctlTermios(int(t.file.Fd()), ioctlSetTermios, &t.orig)
}

func (t *unixTTY) Close() error {
	_ = t.Restore()
	if t.owns {
		return t.file.Close()
	}
	return nil
}

func (t *unixTTY) Interrupt() {
	_ = t.file.SetReadDeadline(time.Now())
}

func (t *unixTTY) ClearDeadline() {
	_ = t.file.SetReadDeadline(time.Time{})
}

func isTerminal(fd int) bool {
	var term syscall.Termios
	return ioctlTermios(fd, ioctlGetTermios, &term) == nil
}

func ioctlTermios(fd int, req uintptr, term *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(term)))
	if errno != 0 {
		return errno
	}
	return nil
}

type winsize struct {
	Row, Col       uint16
	Xpixel, Ypixel uint16
}
