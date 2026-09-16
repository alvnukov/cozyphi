package util

import (
	"strings"
	"unicode/utf8"
)

// NormalizeLF converts CRLF and lone CR line endings to LF.
func NormalizeLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// TruncateRunes cuts s to at most limit runes and marks the cut with an
// ellipsis. Counting runes rather than bytes is what keeps the result valid
// UTF-8: on non-ASCII text a byte limit lands inside a multi-byte character
// about half the time, and the reader gets a replacement glyph where the cut
// was. Use it for text a person or the model reads — a command, a path, a
// description. A string that already fits comes back untouched.
func TruncateRunes(s string, limit int) string {
	return TruncateRunesWith(s, limit, "…")
}

// TruncateRunesWith is TruncateRunes with a marker of the caller's choosing,
// for text where the cut has to say what to do about the missing rest.
func TruncateRunesWith(s string, limit int, marker string) string {
	limit = max(limit, 0)
	count := 0
	for i := range s {
		if count == limit {
			return s[:i] + marker
		}
		count++
	}
	return s
}

// TruncateBytes cuts s to at most limit bytes and marks the cut with an
// ellipsis. The cut backs up to the nearest rune boundary, so a byte budget
// still never splits a character. Reach for it only where bytes are the real
// constraint — a raw response body, a cap on untrusted input — and for
// anything else prefer TruncateRunes. The marker itself is not counted
// against the budget.
func TruncateBytes(s string, limit int) string {
	return TruncateBytesWith(s, limit, "…")
}

// TruncateBytesWith is TruncateBytes with a marker of the caller's choosing.
func TruncateBytesWith(s string, limit int, marker string) string {
	limit = max(limit, 0)
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + marker
}
