package chat

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"
)

// visRow is one display row of the composer after word wrapping: the row text
// plus the cluster/width/offset columns needed to map caret and mouse
// positions back to byte offsets in Value.
type visRow struct {
	text     string
	start    int // byte offset of the first cluster
	end      int // byte offset past the last cluster (before the newline, if hard)
	width    int
	clusters []string
	widths   []int // display width per cluster
	offs     []int // byte offset of each cluster start; len == len(clusters)
	hard     bool  // row ended at a '\n' (not a soft wrap)
}

// layoutEditor wraps value to width, breaking on spaces when possible and
// hard-splitting tokens wider than the row. It is the single source of truth
// for the composer's visual geometry: Draw paints its rows, and caret/mouse
// mapping round-trips through it, so editing and rendering cannot disagree.
func layoutEditor(value string, width int, method xui.WidthMethod) []visRow {
	if width < 1 {
		width = 1
	}
	var rows []visRow
	off := 0
	for line := range strings.SplitSeq(value, "\n") {
		clusters, widths, offs := splitClusters(line, off, method)
		rows = appendLineRows(rows, clusters, widths, offs, width, off)
		off += len(line) + 1 // text plus the newline
	}
	return rows
}

// splitClusters breaks a logical line into grapheme clusters with their
// display widths and absolute byte offsets.
func splitClusters(line string, base int, method xui.WidthMethod) (clusters []string, widths, offs []int) {
	rest := line
	for rest != "" {
		cluster, w, next := xui.FirstGrapheme(rest, method)
		if w < 1 {
			w = 1
		}
		clusters = append(clusters, cluster)
		widths = append(widths, w)
		offs = append(offs, base+len(line)-len(rest))
		rest = next
	}
	return clusters, widths, offs
}

// appendLineRows appends the wrapped rows of one logical line. A wrap happens
// at the last space when the next cluster would overflow; a token wider than
// the row is hard-split. When the space itself is the cluster that overflows,
// it starts the next row — the wrap must still advance past it.
func appendLineRows(
	rows []visRow,
	clusters []string,
	widths, offs []int,
	rowW int,
	base int,
) []visRow {
	start := 0
	brk := -1 // cluster index after which a wrap may occur
	acc := 0
	for i := range clusters {
		cw := widths[i]
		if acc+cw > rowW && i > start {
			end := i
			if brk > start {
				end = brk
			}
			rows = append(rows, makeRow(clusters, widths, offs, start, end, base, false))
			start = end
			acc = sumWidths(widths, start, i)
			brk = -1
		}
		acc += cw
		if isSpaceCluster(clusters[i]) {
			brk = i + 1
		}
	}
	return append(rows, makeRow(clusters, widths, offs, start, len(clusters), base, true))
}

func makeRow(clusters []string, widths, offs []int, from, to, base int, hard bool) visRow {
	r := visRow{hard: hard}
	if to > from {
		var b strings.Builder
		w := 0
		for i := from; i < to; i++ {
			b.WriteString(clusters[i])
			w += widths[i]
		}
		r.text = b.String()
		r.width = w
		r.start = offs[from]
		r.end = offs[to-1] + len(clusters[to-1])
		r.clusters = clusters[from:to]
		r.widths = widths[from:to]
		r.offs = offs[from:to]
	} else {
		r.start = base
		r.end = base
	}
	return r
}

func sumWidths(widths []int, from, to int) int {
	w := 0
	for i := from; i < to; i++ {
		w += widths[i]
	}
	return w
}

func isSpaceCluster(cluster string) bool {
	for _, r := range cluster {
		return unicode.IsSpace(r)
	}
	return false
}

// offsetToRowCol maps a byte offset to its visual row and column. A caret
// at the end of a row that fills the width exactly belongs to the next row
// (column 0) — the terminal cannot render a cursor past the last column.
// A row that soft-wrapped early (at a space, width < rowW) keeps the caret
// on itself at its own last column.
func offsetToRowCol(rows []visRow, off, rowW int) (row, col int) {
	last := len(rows) - 1
	for i := range rows {
		r := &rows[i]
		if off < r.start {
			break
		}
		if off > r.end || (off == r.end && i != last && !r.hard && r.width >= rowW) {
			continue
		}
		cum := 0
		for j := range r.clusters {
			if off <= r.offs[j] {
				return i, cum
			}
			cum += r.widths[j]
		}
		return i, r.width
	}
	return last, rows[last].width
}

// rowColToOffset maps a visual position to a byte offset. A column inside a
// wide (CJK) cluster snaps to that cluster's start — clicks never land mid-glyph.
func rowColToOffset(rows []visRow, row, col int) int {
	if len(rows) == 0 {
		return 0
	}
	if row < 0 {
		row = 0
	}
	if row >= len(rows) {
		row = len(rows) - 1
	}
	r := &rows[row]
	cum := 0
	for j := range r.clusters {
		if col < cum+r.widths[j] {
			return r.offs[j]
		}
		cum += r.widths[j]
	}
	return r.end
}

// rowSelectionCols returns the highlighted column range of a row for the
// selection [selStart, selEnd). Rows fully inside the selection light up
// whole; the newline itself is not a rendered cluster, so it carries no cell.
func rowSelectionCols(r *visRow, selStart, selEnd int) (fromCol, toCol int, ok bool) {
	if selEnd <= r.start || selStart >= r.end {
		return 0, 0, false
	}
	from, to := 0, 0
	for j := range r.clusters {
		if r.offs[j] < selStart {
			from += r.widths[j]
		}
		if r.offs[j] < selEnd {
			to += r.widths[j]
		}
	}
	return from, to, true
}

// prevWordStart moves left past spaces, then past the word, returning the
// offset of the word's first rune.
func prevWordStart(s string, off int) int {
	i := skipLeftWhile(s, off, unicode.IsSpace)
	return skipLeftWhile(s, i, func(r rune) bool { return !unicode.IsSpace(r) })
}

// nextWordEnd moves right past spaces, then past the word.
func nextWordEnd(s string, off int) int {
	i := skipRightWhile(s, off, unicode.IsSpace)
	return skipRightWhile(s, i, func(r rune) bool { return !unicode.IsSpace(r) })
}

func skipLeftWhile(s string, off int, pred func(rune) bool) int {
	i := off
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if !pred(r) {
			break
		}
		i -= size
	}
	return i
}

func skipRightWhile(s string, off int, pred func(rune) bool) int {
	i := off
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !pred(r) {
			break
		}
		i += size
	}
	return i
}
