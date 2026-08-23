package cell

import (
	"unicode"
	"unicode/utf8"
)

// WidthMethod selects how display width is measured.
type WidthMethod int

const (
	// WidthUnicode uses rune-based East Asian width approximation.
	WidthUnicode WidthMethod = iota
	// WidthWCWidth is an alias of WidthUnicode.
	WidthWCWidth
)

// StringWidth returns the display width of s.
func StringWidth(s string, method WidthMethod) int {
	_ = method
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// FirstGrapheme returns the first cluster (one rune) and its display width.
func FirstGrapheme(s string, method WidthMethod) (cluster string, width int, rest string) {
	_ = method
	if s == "" {
		return "", 0, ""
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 1 {
		return s[:1], 1, s[1:]
	}
	// Extend over combining marks.
	end := size
	for end < len(s) {
		r2, sz := utf8.DecodeRuneInString(s[end:])
		if !unicode.Is(unicode.Mn, r2) && !unicode.Is(unicode.Me, r2) && !unicode.Is(unicode.Mc, r2) {
			break
		}
		end += sz
	}
	cluster = s[:end]
	width = runeWidth(r)
	if width < 1 {
		width = 1
	}
	return cluster, width, s[end:]
}

func runeWidth(r rune) int {
	if r == 0 || r == utf8.RuneError {
		return 0
	}
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	// Zero-width / combining
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	// Soft hyphen
	if r == 0x00ad {
		return 1
	}
	// East Asian Wide / Fullwidth ranges (common subset)
	if isWideRune(r) {
		return 2
	}
	return 1
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f: // Hangul Jamo
		return true
	case r == 0x2329 || r == 0x232a:
		return true
	case r >= 0x2e80 && r <= 0xa4cf:
		return true
	case r >= 0xac00 && r <= 0xd7a3: // Hangul Syllables
		return true
	case r >= 0xf900 && r <= 0xfaff:
		return true
	case r >= 0xfe10 && r <= 0xfe19:
		return true
	case r >= 0xfe30 && r <= 0xfe6f:
		return true
	case r >= 0xff00 && r <= 0xff60:
		return true
	case r >= 0xffe0 && r <= 0xffe6:
		return true
	case r >= 0x1f300 && r <= 0x1faff: // emoji block (approx)
		return true
	case r >= 0x20000 && r <= 0x3fffd:
		return true
	default:
		return false
	}
}
