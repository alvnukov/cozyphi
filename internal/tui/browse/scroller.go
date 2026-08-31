package browse

// Scroller is a window over a column of lines with no cursor: the help
// screen, raw text views. The zero value is a window at the top of
// nothing.
type Scroller struct {
	offset   int
	lines    int
	viewport int
}

// SetExtent tells the scroller how many lines exist and how many fit; call
// it from Draw, where both are known, before reading Offset.
func (s *Scroller) SetExtent(lines, viewport int) {
	s.lines, s.viewport = max(lines, 0), max(viewport, 0)
	s.clamp()
}

// Offset is the first visible line.
func (s *Scroller) Offset() int { return s.offset }

// Jump moves the window to put line n first, clamped to the content.
func (s *Scroller) Jump(n int) {
	s.offset = n
	s.clamp()
}

// Apply moves the window by one motion. A page keeps one line of overlap
// so the reader can see the seam.
func (s *Scroller) Apply(m Motion) {
	switch m.Op {
	case OpNone:
		return
	case OpStep:
		s.offset += m.N
	case OpHalfPage:
		s.offset += m.N * max(s.viewport/2, 1)
	case OpPage:
		s.offset += m.N * max(s.viewport-1, 1)
	case OpTop:
		s.offset = 0
	case OpBottom:
		s.offset = s.lines
	case OpIndex:
		s.offset = m.N - 1
	}
	s.clamp()
}

// clamp keeps the window inside the lines, with the last screen flush to
// the bottom.
func (s *Scroller) clamp() {
	s.offset = min(s.offset, max(s.lines-s.viewport, 0))
	s.offset = max(s.offset, 0)
}
