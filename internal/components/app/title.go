package app

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

// terminalTitle owns the last successfully emitted value, not the selected View
// pointer: a retained View can replace its durable session in place.
type terminalTitle struct {
	value   string
	written bool
}

func (t *terminalTitle) sync(root any, write func([]byte) (int, error)) error {
	source, ok := root.(interface{ TerminalTitle() string })
	if !ok {
		return nil
	}
	// Defense at the raw-output boundary: neither OSC terminators nor C1,
	// bidi/format controls, or malformed UTF-8 may reach a terminal payload.
	value := strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, source.TerminalTitle())
	if t.written && t.value == value {
		return nil
	}
	// OSC 0 (icon name + window title) is what several terminals use for the
	// tab title; OSC 2 covers the window title alone. Emit both so the tab
	// follows the selected session wherever the terminal looks.
	sequence := []byte("\x1b]0;" + value + "\x1b\\" + "\x1b]2;" + value + "\x1b\\")
	n, err := write(sequence)
	if err == nil && n != len(sequence) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("set terminal title: %w", err)
	}
	t.value, t.written = value, true
	return nil
}
