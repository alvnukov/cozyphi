package cell

import "github.com/rivo/uniseg"

// WidthMethod selects how display width is measured.
type WidthMethod int

const (
	// WidthUnicode measures display width with Unicode grapheme clusters.
	WidthUnicode WidthMethod = iota
	// WidthWCWidth is an alias of WidthUnicode.
	WidthWCWidth
)

// StringWidth returns the display width of s, measured over grapheme
// clusters — the units a terminal actually renders. A hand-rolled per-rune
// table cannot do this: it counts a ZWJ family emoji as 8 cells, a flag pair
// as two broken halves, and misses emoji presentation via VS16.
func StringWidth(s string, method WidthMethod) int {
	_ = method
	w := 0
	state := -1
	for s != "" {
		var cw int
		_, s, cw, state = uniseg.FirstGraphemeClusterInString(s, state)
		w += cw
	}
	return w
}

// FirstGrapheme returns the first grapheme cluster and its display width.
// A zero-width cluster (a stray combining mark at the string start) still
// reports width 1: callers paint it into a real cell.
func FirstGrapheme(s string, _ WidthMethod) (cluster string, width int, rest string) {
	if s == "" {
		return "", 0, ""
	}
	cluster, rest, width, _ = uniseg.FirstGraphemeClusterInString(s, -1)
	if width < 1 {
		width = 1
	}
	return cluster, width, rest
}
