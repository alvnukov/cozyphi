package cell

// Color represents a terminal cell color.
type Color struct {
	Kind ColorKind
	// Index is used for KindIndex (0-255).
	Index uint8
	// R, G, B are used for KindRGB.
	R, G, B uint8
}

// ColorKind selects how a Color is encoded.
type ColorKind uint8

const (
	ColorDefault ColorKind = iota
	ColorIndex
	ColorRGB
)

// DefaultColor is the terminal default foreground/background.
func DefaultColor() Color {
	return Color{Kind: ColorDefault}
}

// IndexedColor returns an 8/256-color palette color.
func IndexedColor(index uint8) Color {
	return Color{Kind: ColorIndex, Index: index}
}

// RGBColor returns a truecolor RGB color.
func RGBColor(r, g, b uint8) Color {
	return Color{Kind: ColorRGB, R: r, G: g, B: b}
}

// Equal reports whether two colors are identical.
func (c Color) Equal(o Color) bool {
	if c.Kind != o.Kind {
		return false
	}
	switch c.Kind {
	case ColorDefault:
		return true
	case ColorIndex:
		return c.Index == o.Index
	case ColorRGB:
		return c.R == o.R && c.G == o.G && c.B == o.B
	default:
		return false
	}
}
