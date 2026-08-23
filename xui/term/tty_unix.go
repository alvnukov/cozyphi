//go:build unix

package term

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"
)

func openTTY() (TTY, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		if !isTerminal(int(os.Stdin.Fd())) {
			return nil, fmt.Errorf("xui: no tty: %w", err)
		}
		return &unixTTY{file: os.Stdin, out: os.Stdout, fd: int(os.Stdin.Fd())}, nil
	}
	// On Linux os.OpenFile registers the tty with epoll and flips it
	// non-blocking; the raw reads in Read would then spin on EAGAIN.
	// Keep the descriptor blocking: VMIN/VTIME bounds the wait instead.
	_ = syscall.SetNonblock(int(f.Fd()), false)
	return &unixTTY{file: f, out: f, fd: int(f.Fd())}, nil
}

type unixTTY struct {
	file *os.File
	out  *os.File
	// fd is the raw descriptor used by Read: Go never registers a tty with
	// the runtime poller on darwin, so os.File.Read turns a VMIN=0/VTIME
	// timeout into io.EOF (ZeroReadIsEOF) and the loop cannot tell "no data
	// yet" from "terminal gone". The raw syscall keeps them apart.
	fd     int
	owns   bool
	orig   syscall.Termios
	rawSet bool
}

func (t *unixTTY) Read(p []byte) (int, error) {
	n, err := syscall.Read(t.fd, p)
	if n == 0 && err == nil {
		// VMIN=0/VTIME=1 expiry: no data yet, keep reading.
		return 0, nil
	}
	if err == syscall.EIO || err == syscall.EBADF {
		return n, io.EOF
	}
	return n, err
}
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
	// VMIN=0/VTIME=1 bounds every read to 100 ms: the runtime poller never
	// accepts a tty on darwin, so read deadlines cannot interrupt a blocked
	// read and Loop.Stop would hang on wg.Wait. The timer lets the reader
	// notice cancellation; data still returns immediately.
	term.Cc[syscall.VMIN] = 0
	term.Cc[syscall.VTIME] = 1
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
	// Read deadlines never reach a raw-syscall reader (and Go refuses to
	// poll /dev/tty on darwin altogether), so there is nothing to poke:
	// the VMIN=0/VTIME=1 timer is what lets a blocked Read return.
}

func (t *unixTTY) ClearDeadline() {}

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
