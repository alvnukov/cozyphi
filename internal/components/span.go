package components

import (
	"unicode"

	"github.com/pulseaiclub/xui"
)

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

// WrapSpans soft-wraps spans to width, preserving style boundaries and hard
// newlines. Lines break at word boundaries when one fits; a word wider than
// the line breaks at grapheme boundaries so nothing overflows.
func WrapSpans(spans []Span, width int, method xui.WidthMethod) []RichLine {
	if width < 1 {
		width = 1
	}
	var lines []RichLine

	type cluster struct {
		text  string
		style xui.Style
		w     int
		space bool
	}
	var buf []cluster
	bufW := 0
	breakAt := -1 // buf index where the current word starts; -1 = no boundary

	emit := func(cl []cluster) {
		if len(cl) == 0 {
			lines = append(lines, RichLine(nil))
			return
		}
		var line RichLine
		for _, c := range cl {
			if n := len(line); n > 0 && line[n-1].Style == c.style {
				line[n-1].Text += c.text
				continue
			}
			line = append(line, Span{Text: c.text, Style: c.style})
		}
		lines = append(lines, line)
	}

	// reset starts a fresh line buffer.
	reset := func() {
		buf = nil
		bufW = 0
		breakAt = -1
	}

	// splitAt breaks before buf[i]: the head (minus trailing spaces) becomes a
	// line, the remainder stays buffered. A head that trims to nothing emitted
	// no line — the break only ate whitespace.
	splitAt := func(i int) {
		head := buf[:i]
		for len(head) > 0 && head[len(head)-1].space {
			head = head[:len(head)-1]
		}
		if len(head) > 0 {
			emit(head)
		}
		rest := append([]cluster(nil), buf[i:]...)
		buf = rest
		bufW = 0
		for _, c := range buf {
			bufW += c.w
		}
		breakAt = -1
	}

	appendCluster := func(c cluster) {
		// Trailing whitespace never forces a wrap: an overflowing space is
		// held implicitly and just moves the break point past the line.
		if c.space && bufW+c.w > width && bufW > 0 {
			breakAt = len(buf)
			return
		}
		for bufW+c.w > width && bufW > 0 {
			if breakAt > 0 {
				splitAt(breakAt)
			} else {
				emit(buf)
				reset()
			}
		}
		buf = append(buf, c)
		bufW += c.w
		if c.space {
			breakAt = len(buf)
		}
	}

	for _, sp := range spans {
		rest := sp.Text
		for rest != "" {
			if rest[0] == '\n' {
				emit(buf)
				reset()
				rest = rest[1:]
				continue
			}
			clusterText, cw, next := xui.FirstGrapheme(rest, method)
			if cw < 1 {
				cw = 1
			}
			appendCluster(cluster{
				text:  clusterText,
				style: sp.Style,
				w:     cw,
				space: unicode.IsSpace(rune(clusterText[0])),
			})
			rest = next
		}
	}
	if len(buf) > 0 || len(lines) == 0 {
		emit(buf)
	}
	return lines
}

// PaintRichLines paints wrapped lines into a new surface of given width.
func PaintRichLines(width int, lines []RichLine, method xui.WidthMethod, widget Widget) Surface {
	return PaintRichLinesAt(0, width, lines, method, widget)
}

// PaintRichLinesAt paints wrapped lines into a new surface of given width,
// starting each line at column x (an inset rail).
func PaintRichLinesAt(x, width int, lines []RichLine, method xui.WidthMethod, widget Widget) Surface {
	h := len(lines)
	h = max(h, 1)
	s := NewSurface(width, h, widget)
	for y, line := range lines {
		PaintSpans(&s, x, y, line, method)
	}
	return s
}
