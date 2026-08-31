package browse

import "github.com/pulseaiclub/xui"

// Cursor is the selection half of a list browser: a selected row plus the
// scroll window that follows it. Rows may be unselectable — headers,
// blank spacers — and the cursor never rests on one: steps skip them,
// jumps snap to the nearest selectable row in the direction of travel.
type Cursor struct {
	selected   int
	scroll     int
	rows       int
	viewport   int
	selectable func(int) bool
}

// SetRows tells the cursor how many rows exist and which of them can hold
// the selection; a nil selectable means every row can. Call it whenever
// the rows are rebuilt.
func (c *Cursor) SetRows(n int, selectable func(int) bool) {
	c.rows, c.selectable = max(n, 0), selectable
	c.Select(c.selected)
}

// SetViewport tells the cursor how many rows fit; call it from Draw.
func (c *Cursor) SetViewport(h int) {
	c.viewport = max(h, 0)
	c.follow()
}

// Selected is the current row index; zero when there are no rows.
func (c *Cursor) Selected() int { return c.selected }

// Scroll is the first visible row.
func (c *Cursor) Scroll() int { return c.scroll }

// Select puts the cursor on row i, snapping down then up to a selectable
// row, and brings it into view.
func (c *Cursor) Select(i int) { c.jump(i, 1) }

// Apply moves the cursor by one motion; the window follows. A page keeps
// one row of overlap.
func (c *Cursor) Apply(m Motion) {
	switch m.Op {
	case OpNone:
		return
	case OpStep:
		c.MoveBy(m.N)
	case OpHalfPage:
		c.jump(c.selected+m.N*max(c.viewport/2, 1), sign(m.N))
	case OpPage:
		c.jump(c.selected+m.N*max(c.viewport-1, 1), sign(m.N))
	case OpTop:
		c.jump(0, 1)
	case OpBottom:
		c.jump(c.rows-1, -1)
	case OpIndex:
		c.jump(m.N-1, 1)
	}
}

// MoveBy steps the cursor delta selectable rows — each unit lands on the
// next selectable row, however many spacers sit between. The cursor stays
// put at an edge.
func (c *Cursor) MoveBy(delta int) {
	dir := sign(delta)
	for range max(delta*dir, 0) {
		next, ok := c.nearest(c.selected+dir, dir)
		if !ok {
			break
		}
		c.selected = next
	}
	c.follow()
}

// Wheel scrolls the window and leaves the cursor where it is, even
// offscreen; keyboard motions bring it back.
func (c *Cursor) Wheel(e xui.MouseEvent) bool {
	m, ok := Wheel(e)
	if !ok {
		return false
	}
	c.scroll += m.N
	c.clampScroll()
	return true
}

// jump lands on target, snapping to a selectable row toward dir first and
// away from it second, and brings the cursor into view.
func (c *Cursor) jump(target, dir int) {
	target = min(max(target, 0), max(c.rows-1, 0))
	if i, ok := c.nearest(target, dir); ok {
		c.selected = i
	} else if i, ok := c.nearest(target, -dir); ok {
		c.selected = i
	} else {
		c.selected = target
	}
	c.follow()
}

// nearest walks from i in direction dir to the first selectable row,
// including i itself.
func (c *Cursor) nearest(i, dir int) (int, bool) {
	for ; i >= 0 && i < c.rows; i += dir {
		if c.selectable == nil || c.selectable(i) {
			return i, true
		}
	}
	return 0, false
}

// follow keeps the selected row inside the window.
func (c *Cursor) follow() {
	c.scroll = min(c.scroll, c.selected)
	c.scroll = max(c.scroll, c.selected-max(c.viewport, 1)+1)
	c.clampScroll()
}

// clampScroll keeps the window inside the rows, with the last screen
// flush to the bottom.
func (c *Cursor) clampScroll() {
	c.scroll = min(c.scroll, max(c.rows-c.viewport, 0))
	c.scroll = max(c.scroll, 0)
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
