package screen

import "github.com/pulseaiclub/xui/cell"

// Window is a logical clipped viewport into a Screen.
type Window struct {
	screen *Screen
	x, y   int // origin in parent/screen coordinates
	width  int
	height int
}

// NewWindow wraps an entire screen.
func NewWindow(s *Screen) Window {
	w, h := s.Size()
	return Window{screen: s, width: w, height: h}
}

// Size returns the window dimensions.
func (w Window) Size() (width, height int) {
	return w.width, w.height
}

// Origin returns the window origin on the screen.
func (w Window) Origin() (x, y int) {
	return w.x, w.y
}

// Child returns a clipped child window.
func (w Window) Child(x, y, width, height int) Window {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if x < 0 {
		width += x
		x = 0
	}
	if y < 0 {
		height += y
		y = 0
	}
	if x+width > w.width {
		width = w.width - x
	}
	if y+height > w.height {
		height = w.height - y
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return Window{
		screen: w.screen,
		x:      w.x + x,
		y:      w.y + y,
		width:  width,
		height: height,
	}
}

// Clear fills the window with empty cells.
func (w Window) Clear() {
	empty := cell.EmptyCell()
	empty.Default = false
	for row := 0; row < w.height; row++ {
		for col := 0; col < w.width; col++ {
			w.screen.SetCell(w.x+col, w.y+row, empty)
		}
	}
}

// Fill fills the window with a character and style.
func (w Window) Fill(ch string, style cell.Style) {
	c := cell.Cell{Char: ch, Width: 1, Style: style}
	for row := 0; row < w.height; row++ {
		for col := 0; col < w.width; col++ {
			w.SetCell(col, row, c)
		}
	}
}

// SetCell writes a cell in window-local coordinates.
func (w Window) SetCell(x, y int, c cell.Cell) {
	if x < 0 || y < 0 || x >= w.width || y >= w.height {
		return
	}
	w.screen.SetCell(w.x+x, w.y+y, c)
}

// GetCell reads a cell in window-local coordinates.
func (w Window) GetCell(x, y int) cell.Cell {
	if x < 0 || y < 0 || x >= w.width || y >= w.height {
		return cell.EmptyCell()
	}
	return w.screen.GetCell(w.x+x, w.y+y)
}

// Print writes text starting at (x,y) with the given style.
// Returns the number of columns advanced.
func (w Window) Print(x, y int, text string, style cell.Style) int {
	return w.PrintWithMethod(x, y, text, style, cell.WidthUnicode)
}

// PrintWithMethod is Print with an explicit width method.
func (w Window) PrintWithMethod(x, y int, text string, style cell.Style, method cell.WidthMethod) int {
	if y < 0 || y >= w.height {
		return 0
	}
	col := x
	rest := text
	for rest != "" && col < w.width {
		cluster, width, next := cell.FirstGrapheme(rest, method)
		rest = next
		if width < 1 {
			width = 1
		}
		if col < 0 {
			col += width
			continue
		}
		if col+width > w.width {
			break
		}
		w.SetCell(col, y, cell.Cell{
			Char:  cluster,
			Width: uint8(width),
			Style: style,
		})
		col += width
	}
	if col < x {
		return 0
	}
	return col - x
}
