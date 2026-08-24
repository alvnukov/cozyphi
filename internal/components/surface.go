package components

import (
	"slices"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"
)

// Size is a 2D size in cells.
type Size struct {
	Width, Height int
}

// Point is a 2D coordinate.
type Point struct {
	X, Y int
}

// Constraints mirrors Flutter-style layout constraints.
type Constraints struct {
	Min, Max Size
}

// Tight returns fixed-size constraints.
func Tight(w, h int) Constraints {
	s := Size{Width: w, Height: h}
	return Constraints{Min: s, Max: s}
}

// EventContext carries event-handling side effects.
type EventContext struct {
	Redraw  bool
	Quit    bool
	Consume bool
	Focus   Widget
}

// ConsumeAndRedraw marks the event handled and requests a redraw.
func (c *EventContext) ConsumeAndRedraw() {
	c.Consume = true
	c.Redraw = true
}

// RequestFocus queues a focus change.
func (c *EventContext) RequestFocus(w Widget) {
	c.Focus = w
}

// DrawContext is passed to Widget.Draw.
type DrawContext struct {
	Min, Max Size
	Method   xui.WidthMethod
	// Wake, when non-nil, collects the earliest future instant at which the
	// drawing tree wants another frame. Widgets that animate call WakeAt /
	// WakeIn on every Draw pass where they need to keep animating; stopping
	// is implicit (the Draw call stops happening). Nil in tests and
	// standalone draws: publishing is then a no-op.
	Wake *time.Time
}

// WithConstraints returns a child draw context.
func (d DrawContext) WithConstraints(minVal, maxVal Size) DrawContext {
	return DrawContext{Min: minVal, Max: maxVal, Method: d.Method, Wake: d.Wake}
}

// WakeAt schedules a redraw at t, keeping the earliest requested time.
// Safe on a zero DrawContext (no-op). A zero t is ignored.
func (d DrawContext) WakeAt(t time.Time) {
	if d.Wake == nil || t.IsZero() {
		return
	}
	if d.Wake.IsZero() || t.Before(*d.Wake) {
		*d.Wake = t
	}
}

// WakeIn is WakeAt(now + d).
func (d DrawContext) WakeIn(dd time.Duration) {
	d.WakeAt(time.Now().Add(dd))
}

// Widget is a focusable, drawable UI node.
type Widget interface {
	Handle(ctx *EventContext, ev xui.Event)
	Draw(ctx DrawContext) Surface
}

// Surface is one frame of laid-out widget output.
type Surface struct {
	Size     Size
	Buffer   []xui.Cell // row-major, len == Width*Height; may be nil for container-only
	Children []SubSurface
	Widget   Widget // identity for focus/hit-test
	// Cursor is an optional screen-local cursor hint (set by leaf widgets).
	Cursor *Point
}

// SubSurface places a child surface inside a parent.
type SubSurface struct {
	Origin  Point
	Z       int
	Surface Surface
}

// NewSurface allocates a blank cell buffer.
func NewSurface(w, h int, widget Widget) Surface {
	buf := make([]xui.Cell, w*h)
	for i := range buf {
		buf[i] = xui.EmptyCell()
	}
	return Surface{Size: Size{Width: w, Height: h}, Buffer: buf, Widget: widget}
}

// CloneSurface returns a deep copy whose buffers and child surfaces can be
// styled without mutating an immutable cached frame.
func CloneSurface(s Surface) Surface {
	out := s
	out.Buffer = append([]xui.Cell(nil), s.Buffer...)
	out.Children = make([]SubSurface, len(s.Children))
	for i, child := range s.Children {
		out.Children[i] = child
		out.Children[i].Surface = CloneSurface(child.Surface)
	}
	if s.Cursor != nil {
		cursor := *s.Cursor
		out.Cursor = &cursor
	}
	return out
}

// SetCell writes into the surface buffer.
// Wide glyphs (Width>1) also fill trailing columns so a later full-grid paint
// cannot leave stale single-width spaces under CJK cells.
func (s *Surface) SetCell(x, y int, c xui.Cell) {
	if s.Buffer == nil || x < 0 || y < 0 || x >= s.Size.Width || y >= s.Size.Height {
		return
	}
	if c.Width == 0 {
		c.Width = 1
	}
	if c.Char != "" {
		if disp := xui.StringWidth(c.Char, xui.WidthUnicode); disp > int(c.Width) {
			c.Width = uint8(disp) //nolint:gosec // G115: display width is a small cell count
		}
	}
	if c.Width > 1 {
		c.Trail = false
	}
	s.Buffer[y*s.Size.Width+x] = c
	if c.Width > 1 {
		for i := 1; i < int(c.Width); i++ {
			if x+i >= s.Size.Width {
				break
			}
			// Continuation column: Render steps over it; Trail marks it so a
			// mistaken per-column paint cannot wipe the CJK glyph on the tty.
			s.Buffer[y*s.Size.Width+x+i] = xui.Cell{
				Char:  " ",
				Width: 1,
				Trail: true,
				// Do not copy Reverse — cursor reverse on a wide glyph must not
				// leave a reverse trail cell in the buffer.
				Style: xui.Style{Fg: c.Style.Fg, Bg: c.Style.Bg},
			}
		}
	}
}

// Print writes text into the surface and returns columns advanced.
func (s *Surface) Print(x, y int, text string, style xui.Style, method xui.WidthMethod) int {
	col := x
	rest := text
	for rest != "" && col < s.Size.Width {
		cluster, width, next := xui.FirstGrapheme(rest, method)
		rest = next
		if width < 1 {
			width = 1
		}
		if col+width > s.Size.Width {
			break
		}
		s.SetCell(col, y, xui.Cell{Char: cluster, Width: uint8(width), Style: style})
		col += width
	}
	if col < x {
		return 0
	}
	return col - x
}

// Render paints this surface tree into a Window and returns the absolute cursor, if any.
// Cursor coordinates are in the same space as win (screen-absolute for the root window).
func (s Surface) Render(win xui.Window) *Point {
	return renderSurface(s, win, 0, 0)
}

func renderSurface(s Surface, win xui.Window, ox, oy int) *Point {
	if s.Buffer != nil {
		for y := 0; y < s.Size.Height; y++ {
			// Step by cell width so wide glyphs are not followed by a paint of
			// their continuation column (which would create gaps between CJK glyphs /
			// or fake block cursors after a background fill).
			for x := 0; x < s.Size.Width; {
				c := s.Buffer[y*s.Size.Width+x]
				step := int(c.Width)
				step = max(step, 1)
				// Skip Default and Trail pads. Emitting Trail spaces to the
				// screen/tty clears the preceding wide glyph.
				if !c.Default && !c.Trail {
					win.SetCell(ox+x, oy+y, c)
				}
				x += step
			}
		}
	}
	// Simple z-order: stable sort by Z ascending.
	children := append([]SubSurface(nil), s.Children...)
	for i := range len(children) {
		for j := i + 1; j < len(children); j++ {
			if children[j].Z < children[i].Z {
				children[i], children[j] = children[j], children[i]
			}
		}
	}
	var cursor *Point
	if s.Cursor != nil {
		lx, ly := ox+s.Cursor.X, oy+s.Cursor.Y
		ww, wh := win.Size()
		if lx >= 0 && ly >= 0 && lx < ww && ly < wh {
			wx, wy := win.Origin()
			c := Point{X: wx + lx, Y: wy + ly}
			cursor = &c
		}
	}
	// Clip children to this surface's box so scrolled content cannot paint
	// into siblings (tui/footer). ScrollView / MessageList rely on this.
	clip := win
	if s.Size.Width > 0 && s.Size.Height > 0 {
		clip = win.Child(ox, oy, s.Size.Width, s.Size.Height)
	}
	for _, ch := range children {
		if c := renderSurface(ch.Surface, clip, ch.Origin.X, ch.Origin.Y); c != nil {
			cursor = c
		}
	}
	return cursor
}

// HitTest finds the deepest widget at (x,y) in surface-local coords.
func (s Surface) HitTest(x, y int) Widget {
	w, _, _ := s.HitTestAt(x, y)
	return w
}

// HitTestAt returns the deepest widget and coordinates local to that widget's surface.
func (s Surface) HitTestAt(x, y int) (Widget, int, int) {
	for i := range slices.Backward(s.Children) {
		ch := s.Children[i]
		lx := x - ch.Origin.X
		ly := y - ch.Origin.Y
		if lx >= 0 && ly >= 0 && lx < ch.Surface.Size.Width && ly < ch.Surface.Size.Height {
			if w, wx, wy := ch.Surface.HitTestAt(lx, ly); w != nil {
				return w, wx, wy
			}
		}
	}
	if s.Widget != nil && x >= 0 && y >= 0 && x < s.Size.Width && y < s.Size.Height {
		return s.Widget, x, y
	}
	return nil, 0, 0
}

// SurfaceText extracts text content from a surface and its children for testing/debugging.
// Wide glyphs are emitted once; continuation padding columns are skipped.
func SurfaceText(s Surface) string {
	var b strings.Builder
	for y := 0; y < s.Size.Height; y++ {
		for x := 0; s.Buffer != nil && x < s.Size.Width; {
			c := s.Buffer[y*s.Size.Width+x]
			step := int(c.Width)
			step = max(step, 1)
			ch := c.Char
			if ch == "" {
				ch = " "
			}
			b.WriteString(ch)
			x += step
		}
		b.WriteByte('\n')
	}
	for _, ch := range s.Children {
		b.WriteString(SurfaceText(ch.Surface))
	}
	return b.String()
}
