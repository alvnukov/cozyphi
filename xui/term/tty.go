package term

// TTY abstracts raw terminal I/O.
type TTY interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Size() (cols, rows int, err error)
	MakeRaw() error
	Restore() error
	Fd() uintptr
	Close() error
	// Interrupt unblocks a stuck Read (e.g. via read deadline).
	Interrupt()
}

// OpenTTY opens the controlling terminal.
func OpenTTY() (TTY, error) {
	return openTTY()
}
