package screen

import "github.com/pulseaiclub/xui/cell"

// Screen holds a double-buffered cell grid (front/back swap).
type Screen struct {
	width, height int
	front, back   []cell.Cell
	fullRefresh   bool
	cursorX       int
	cursorY       int
	cursorVisible bool
	cursorShape   int
}

// NewScreen creates a screen of the given size.
func NewScreen(width, height int) *Screen {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	n := width * height
	s := &Screen{
		width:       width,
		height:      height,
		front:       make([]cell.Cell, n),
		back:        make([]cell.Cell, n),
		fullRefresh: true,
	}
	for i := range s.front {
		s.front[i] = cell.EmptyCell()
		s.back[i] = cell.EmptyCell()
	}
	return s
}

// Size returns the screen dimensions.
func (s *Screen) Size() (width, height int) {
	return s.width, s.height
}

// Resize rebuilds both buffers and marks a full refresh.
func (s *Screen) Resize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if width == s.width && height == s.height {
		return
	}
	n := width * height
	front := make([]cell.Cell, n)
	back := make([]cell.Cell, n)
	for i := range front {
		front[i] = cell.EmptyCell()
		back[i] = cell.EmptyCell()
	}
	s.width = width
	s.height = height
	s.front = front
	s.back = back
	s.fullRefresh = true
}

func (s *Screen) idx(x, y int) int {
	return y*s.width + x
}

// Back returns the drawable back buffer.
func (s *Screen) Back() []cell.Cell {
	return s.back
}

// Clear resets the back buffer to empty cells (skipping already-default cells).
func (s *Screen) Clear() {
	for i := range s.back {
		if s.back[i].Default {
			continue
		}
		s.back[i] = cell.EmptyCell()
	}
}

// SetCell writes a cell into the back buffer.
func (s *Screen) SetCell(x, y int, c cell.Cell) {
	if x < 0 || y < 0 || x >= s.width || y >= s.height {
		return
	}
	if c.Width == 0 {
		c.Width = 1
	}
	if c.Char == "" {
		c.Char = " "
	}
	// Keep Width consistent with the glyph so Diff steps over continuation
	// columns and the renderer does not desync on CJK.
	if disp := cell.StringWidth(c.Char, cell.WidthUnicode); disp > int(c.Width) {
		c.Width = uint8(disp)
	}
	c.Default = false
	// A caller may pass Trail by mistake on a primary; only continuations set it.
	if c.Width > 1 {
		c.Trail = false
	}
	s.back[s.idx(x, y)] = c
	if c.Width > 1 {
		for i := 1; i < int(c.Width); i++ {
			if x+i >= s.width {
				break
			}
			cont := cell.EmptyCell()
			cont.Default = false
			cont.Char = " "
			cont.Width = 1
			cont.Trail = true
			cont.Style = cell.Style{Fg: c.Style.Fg, Bg: c.Style.Bg}
			s.back[s.idx(x+i, y)] = cont
		}
	}
}

// GetCell reads from the back buffer.
func (s *Screen) GetCell(x, y int) cell.Cell {
	if x < 0 || y < 0 || x >= s.width || y >= s.height {
		return cell.EmptyCell()
	}
	return s.back[s.idx(x, y)]
}

// MarkRefresh forces a full redraw on the next Diff.
func (s *Screen) MarkRefresh() {
	s.fullRefresh = true
}

// SetCursor sets the cursor position and makes it visible.
func (s *Screen) SetCursor(x, y int) {
	s.cursorX = x
	s.cursorY = y
	s.cursorVisible = true
}

// ClearCursor hides the cursor.
func (s *Screen) ClearCursor() {
	s.cursorVisible = false
}

// Cursor returns cursor state.
func (s *Screen) Cursor() (x, y int, visible bool, shape int) {
	return s.cursorX, s.cursorY, s.cursorVisible, s.cursorShape
}

// SetCursorShape sets DECSCUSR shape (0 = default).
func (s *Screen) SetCursorShape(shape int) {
	s.cursorShape = shape
}

// Diff returns cells that differ between front (displayed) and back (drawn).
//
// Damage is tracked at row granularity: if any cell on a row changes, the
// entire row is emitted. Cell-level partial updates leave stale glyphs on the
// TTY after CJK width churn or fast scroll (vertical "ghost columns" of
// leftover ASCII such as ')', 's', 'd'). Rewriting the whole row clears them.
func (s *Screen) Diff() []cell.DirtyCell {
	out := make([]cell.DirtyCell, 0, 64)
	if s.fullRefresh {
		for y := 0; y < s.height; y++ {
			out = append(out, s.emitRow(y)...)
		}
		s.fullRefresh = false
		return out
	}
	for y := 0; y < s.height; y++ {
		if s.rowDamaged(y) {
			out = append(out, s.emitRow(y)...)
		}
	}
	return out
}

// emitRow appends every non-trail cell on row y from the back buffer.
func (s *Screen) emitRow(y int) []cell.DirtyCell {
	out := make([]cell.DirtyCell, 0, s.width)
	for x := 0; x < s.width; {
		c := s.back[s.idx(x, y)]
		step := int(c.Width)
		if step < 1 {
			step = 1
		}
		// Never emit trail pads — see Cell.Trail.
		if !c.Trail {
			out = append(out, cell.DirtyCell{X: x, Y: y, Cell: c})
		}
		x += step
	}
	return out
}

// rowDamaged reports whether front and back differ anywhere on row y.
func (s *Screen) rowDamaged(y int) bool {
	for x := 0; x < s.width; {
		fi := s.idx(x, y)
		front := s.front[fi]
		back := s.back[fi]
		fw := int(front.Width)
		if fw < 1 {
			fw = 1
		}
		bw := int(back.Width)
		if bw < 1 {
			bw = 1
		}
		// Skip walking from a trail column; snap is handled by width steps
		// from primaries. If we landed here, advance one.
		if back.Trail && front.Trail && front.Equal(back) {
			x++
			continue
		}
		if front.Equal(back) && fw == bw && !back.Trail {
			x += bw
			continue
		}
		return true
	}
	return false
}

// Present swaps front and back buffers after a successful write.
func (s *Screen) Present() {
	s.front, s.back = s.back, s.front
}
