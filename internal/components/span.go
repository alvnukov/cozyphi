package components

import "github.com/pulseaiclub/xui"

// Span is a styled run of text.
type Span struct {
	Text  string
	Style xui.Style
}

// RichLine is one visual line of spans.
type RichLine []Span

// MeasureSpans returns display width of spans.
func MeasureSpans(spans []Span, method xui.WidthMethod) int {
	w := 0
	for _, s := range spans {
		w += xui.StringWidth(s.Text, method)
	}
	return w
}

// PaintSpans writes spans starting at (x,y) on surface; returns columns advanced.
func PaintSpans(s *Surface, x, y int, spans []Span, method xui.WidthMethod) int {
	col := x
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		before := col
		// Advance by what Print actually painted — StringWidth can disagree
		// (e.g. clipped at edge, or control runes) and leave Default holes that
		// survive as blank gaps after win.Clear().
		adv := s.Print(col, y, sp.Text, sp.Style, method)
		col += adv
		if col == before && sp.Text != "" {
			col++
		}
	}
	return col - x
}

// WrapSpans soft-wraps spans to width, preserving style boundaries and hard newlines.
func WrapSpans(spans []Span, width int, method xui.WidthMethod) []RichLine {
	if width < 1 {
		width = 1
	}
	var lines []RichLine
	var cur RichLine
	curW := 0

	flush := func() {
		lines = append(lines, cur)
		cur = nil
		curW = 0
	}

	appendCluster := func(cluster string, style xui.Style, cw int) {
		if curW+cw > width && curW > 0 {
			flush()
		}
		cur = append(cur, Span{Text: cluster, Style: style})
		curW += cw
	}

	for _, sp := range spans {
		rest := sp.Text
		for rest != "" {
			if rest[0] == '\n' {
				flush()
				rest = rest[1:]
				continue
			}
			cluster, cw, next := xui.FirstGrapheme(rest, method)
			if cw < 1 {
				cw = 1
			}
			appendCluster(cluster, sp.Style, cw)
			rest = next
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// PaintRichLines paints wrapped lines into a new surface of given width.
func PaintRichLines(width int, lines []RichLine, method xui.WidthMethod, widget Widget) Surface {
	h := len(lines)
	h = max(h, 1)
	s := NewSurface(width, h, widget)
	for y, line := range lines {
		PaintSpans(&s, 0, y, line, method)
	}
	return s
}
