package text

import (
	"unicode"
	"unicode/utf8"
)

// PrevWordStart moves left past spaces, then past the word, returning the
// offset of the word's first rune.
func PrevWordStart(s string, off int) int {
	i := SkipLeftWhile(s, off, unicode.IsSpace)
	return SkipLeftWhile(s, i, func(r rune) bool { return !unicode.IsSpace(r) })
}

// NextWordEnd moves right past spaces, then past the word.
func NextWordEnd(s string, off int) int {
	i := SkipRightWhile(s, off, unicode.IsSpace)
	return SkipRightWhile(s, i, func(r rune) bool { return !unicode.IsSpace(r) })
}

// SkipLeftWhile walks left from off while pred holds, landing on a rune boundary.
func SkipLeftWhile(s string, off int, pred func(rune) bool) int {
	i := clampOffset(s, off)
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if !pred(r) {
			break
		}
		i -= size
	}
	return i
}

// SkipRightWhile walks right from off while pred holds, landing on a rune boundary.
func SkipRightWhile(s string, off int, pred func(rune) bool) int {
	i := clampOffset(s, off)
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !pred(r) {
			break
		}
		i += size
	}
	return i
}

// clampOffset keeps off inside s: these take a caller's cursor, and a cursor
// left stale by an edit must not slice out of range mid-keystroke.
func clampOffset(s string, off int) int {
	if off < 0 {
		return 0
	}
	if off > len(s) {
		return len(s)
	}
	return off
}
