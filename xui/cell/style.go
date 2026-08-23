package cell

// Style describes cell visual attributes.
type Style struct {
	Fg, Bg        Color
	Bold          bool
	Dim           bool
	Italic        bool
	Underline     bool
	Strikethrough bool
	Reverse       bool
}

// Equal reports whether two styles are identical.
func (s Style) Equal(o Style) bool {
	return s.Fg.Equal(o.Fg) &&
		s.Bg.Equal(o.Bg) &&
		s.Bold == o.Bold &&
		s.Dim == o.Dim &&
		s.Italic == o.Italic &&
		s.Underline == o.Underline &&
		s.Strikethrough == o.Strikethrough &&
		s.Reverse == o.Reverse
}

func (c Color) equal(o Color) bool { return c.Equal(o) }
