package chat

import "github.com/rivo/uniseg"

// Use Unicode grapheme boundaries so an editing command cannot leave half an
// emoji or an orphaned combining mark. xui's helper only groups combining marks.
func nextGrapheme(s string, off int) int {
	if off >= len(s) {
		return len(s)
	}
	cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(s[off:], -1)
	return off + len(cluster)
}

func prevGrapheme(s string, off int) int {
	if off <= 0 {
		return 0
	}
	// ASCII cannot be the trailing code point of a multi-code-point grapheme
	// in sanitized composer text (which contains no CR).
	if s[off-1] < 0x80 {
		return off - 1
	}
	start := lineStart(s, off)
	for start < off {
		next := nextGrapheme(s, start)
		if next >= off {
			return start
		}
		start = next
	}
	return start
}
