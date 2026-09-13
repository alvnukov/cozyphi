package tooltip

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

// TooltipMaxWidth caps the hint panel: wide enough for a sentence, narrow
// enough to float over the control it explains without hiding its neighbors.
const TooltipMaxWidth = 40

// tooltipMaxLines keeps a hint a hint; longer texts are cut with an ellipsis.
const tooltipMaxLines = 6

// tooltipZ floats the hint above every other layer (palette 10, toast 50):
// it answers the pointer directly and must not be covered. The panel has no
// Widget, so hit tests fall through it to the control underneath — clicks
// keep reaching what the user sees the hint about.
const tooltipZ = 60

// PlaceTooltip wraps hint text and anchors the panel next to at, in screen
// coordinates: right of and above the pointer first, flipping below and
// clamping at the edges so the panel never leaves the terminal. The chrome
// is the shared panel language — solid BackgroundPanel fill, rounded Border
// frame, first line in Foreground and the rest Muted. ok is false when the
// screen is too small to hold a hint.
func PlaceTooltip(
	text string,
	at components.Point,
	maxW, maxH int,
	th components.Theme,
	method xui.WidthMethod,
) (components.SubSurface, bool) {
	width := min(TooltipMaxWidth, maxW-2)
	if width < 8 || maxH < 3 {
		return components.SubSurface{}, false
	}
	lines := wrapHint(text, width, method)
	if len(lines) == 0 {
		return components.SubSurface{}, false
	}
	// The height cap leaves room for the frame rows; the cut line says so.
	if len(lines) > tooltipMaxLines || len(lines) > maxH-2 {
		lines = lines[:min(tooltipMaxLines, maxH-2)]
		lines[len(lines)-1] = layout.EllipsizeToWidth(strings.TrimRight(lines[len(lines)-1], " "), width, method)
	}

	textW := 0
	for _, line := range lines {
		textW = max(textW, xui.StringWidth(line, method))
	}
	boxW, boxH := textW+2, len(lines)+2

	panel := components.NewSurface(boxW, boxH, nil)
	fill := xui.Style{Fg: th.Foreground.Fg, Bg: th.BackgroundPanel.Bg}
	for y := range boxH {
		for x := range boxW {
			panel.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, th.Border, nil, nil, nil, nil, method)
	for i, line := range lines {
		style := fill
		if i > 0 {
			style.Fg = th.Muted.Fg
		}
		panel.Print(1, 1+i, line, style, method)
	}

	return components.SubSurface{Origin: anchorTooltip(at, boxW, boxH, maxW, maxH), Surface: panel, Z: tooltipZ}, true
}

// anchorTooltip places the panel right of and one row above the pointer,
// flipping below when the top edge is near and shifting back inside at the
// right and bottom edges, so the hint never covers the pointer cell itself.
func anchorTooltip(at components.Point, boxW, boxH, maxW, maxH int) components.Point {
	x := at.X + 2
	if x+boxW > maxW {
		x = maxW - boxW
	}
	x = max(x, 0)
	y := at.Y - boxH - 1
	if y < 0 {
		y = at.Y + 1
	}
	if y+boxH > maxH {
		y = maxH - boxH
	}
	return components.Point{X: x, Y: max(y, 0)}
}

// wrapHint breaks text into display lines of at most width cells: explicit
// newlines start a fresh line, words wrap on spaces, and a word wider than
// the line wraps at the glyph level so URLs and paths stay readable.
func wrapHint(text string, width int, method xui.WidthMethod) []string {
	var out []string
	for para := range strings.SplitSeq(text, "\n") {
		line := ""
		for word := range strings.FieldsSeq(para) {
			if line != "" && xui.StringWidth(line+" "+word, method) <= width {
				line += " " + word
				continue
			}
			if line != "" {
				out = append(out, line)
			}
			for xui.StringWidth(word, method) > width {
				head, rest := splitAtWidth(word, width, method)
				out = append(out, head)
				word = rest
			}
			line = word
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// splitAtWidth cuts s at the last glyph boundary that fits in width cells
// (at least one glyph), returning the head and the remainder.
func splitAtWidth(s string, width int, method xui.WidthMethod) (head, rest string) {
	w := 0
	for rest = s; rest != ""; {
		cluster, cw, next := xui.FirstGrapheme(rest, method)
		if w > 0 && w+max(cw, 1) > width {
			return head, rest
		}
		head += cluster
		w += max(cw, 1)
		rest = next
	}
	return head, ""
}
