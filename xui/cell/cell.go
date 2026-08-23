package cell

// Hyperlink is an OSC 8 hyperlink attachment.
type Hyperlink struct {
	URI string
	ID  string
}

// Equal reports whether two hyperlinks match.
func (h Hyperlink) Equal(o Hyperlink) bool {
	return h.URI == o.URI && h.ID == o.ID
}

// Empty reports whether the hyperlink has no URI.
func (h Hyperlink) Empty() bool {
	return h.URI == ""
}

// Cell is one grid cell on the screen.
type Cell struct {
	Char      string
	Width     uint8
	Style     Style
	Hyperlink Hyperlink
	// Default is true when the cell is an untouched blank cell.
	Default bool
	// Trail is true for a wide-glyph continuation column. These must never be
	// written to the tty: a space on the trail column clears the whole glyph
	// in VTE/xterm-class terminals (looks like "missing" CJK with a blank gap).
	Trail bool
}

// EmptyCell returns a default blank cell.
func EmptyCell() Cell {
	return Cell{
		Char:    " ",
		Width:   1,
		Default: true,
	}
}

// Equal reports whether two cells are visually identical.
func (c Cell) Equal(o Cell) bool {
	if c.Default && o.Default {
		return true
	}
	return c.Char == o.Char &&
		c.Width == o.Width &&
		c.Trail == o.Trail &&
		c.Style.Equal(o.Style) &&
		c.Hyperlink.Equal(o.Hyperlink)
}

// DirtyCell is a cell that needs to be written to the terminal.
type DirtyCell struct {
	X, Y int
	Cell Cell
}
