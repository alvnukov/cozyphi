package text

import (
	"strings"

	"github.com/pulseaiclub/xui"
)

// WrapEditorLines soft-wraps text to width for display (hard newlines preserved).
func WrapEditorLines(text string, width int, method xui.WidthMethod) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for para := range strings.SplitSeq(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		rest := para
		for rest != "" {
			line := ""
			w := 0
			for rest != "" {
				cluster, cw, next := xui.FirstGrapheme(rest, method)
				if cw < 1 {
					cw = 1
				}
				if w+cw > width && w > 0 {
					break
				}
				line += cluster
				w += cw
				rest = next
				if w >= width {
					break
				}
			}
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func CursorLineCol(text string, cursor, width int, method xui.WidthMethod) (line, col int) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}
	before := text[:cursor]
	lines := WrapEditorLines(before, width, method)
	// WrapEditorLines on prefix may end with an extra empty visual line when
	// cursor is right after a newline — that's correct.
	line = len(lines) - 1
	if line < 0 {
		return 0, 0
	}
	col = xui.StringWidth(lines[line], method)
	// Soft-wrap boundary: a full-width visual line means the caret sits on the
	// next row. Clamping to width-1 would land on a CJK continuation column.
	if col >= width {
		return line + 1, 0
	}
	return line, col
}

// SnapSurfaceColToGlyphStart moves col left if it sits inside a wide glyph's
// trailing columns (Width>1 primary to the left).
func SnapSurfaceColToGlyphStart(buf []xui.Cell, rowW, col, row int) int {
	if buf == nil || rowW < 1 || col < 0 || row < 0 {
		return col
	}
	if row*rowW >= len(buf) {
		return col
	}
	x := 0
	for x < rowW {
		i := row*rowW + x
		if i >= len(buf) {
			break
		}
		step := int(buf[i].Width)
		if step < 1 {
			step = 1
		}
		if col >= x && col < x+step {
			return x
		}
		x += step
	}
	return col
}
