package components

import (
	"strings"
	"unicode"
)

// PlainText strips terminal escape payloads from untrusted display text.
// It is a display projection: saved output stays raw. Incomplete sequences are
// discarded; Unicode joiners are retained so ordinary graphemes stay intact.
func PlainText(value string) string {
	const (
		textState = iota
		escapeState
		csiState
		payloadState
		payloadEscapeState
		intermediateState
	)
	var out strings.Builder
	out.Grow(len(value))
	state := textState
	for _, r := range value {
		switch state {
		case escapeState:
			switch {
			case r == '[':
				state = csiState
			case r == ']' || r == 'P' || r == '^' || r == '_':
				state = payloadState
			case r == '\x1b':
				state = escapeState
			case r >= 0x20 && r <= 0x2f:
				state = intermediateState
			default:
				state = textState
			}
		case intermediateState:
			if r == '\x1b' {
				state = escapeState
			} else if r >= 0x30 && r <= 0x7e {
				state = textState
			}
		case csiState:
			if r == '\x1b' {
				state = escapeState
			} else if r >= 0x40 && r <= 0x7e {
				state = textState
			}
		case payloadState:
			switch r {
			case '\a', '\u009c':
				state = textState
			case '\x1b':
				state = payloadEscapeState
			}
		case payloadEscapeState:
			if r == '\\' || r == '\a' || r == '\u009c' {
				state = textState
			} else if r != '\x1b' {
				state = payloadState
			}
		default:
			switch r {
			case '\x1b':
				state = escapeState
			case '\u009b':
				state = csiState
			case '\u0090', '\u009d', '\u009e', '\u009f':
				state = payloadState
			case '\n':
				out.WriteRune(r)
			case '\t':
				out.WriteRune(' ')
			default:
				if !unicode.IsControl(r) {
					out.WriteRune(r)
				}
			}
		}
	}
	return out.String()
}
