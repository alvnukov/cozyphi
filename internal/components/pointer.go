package components

// Pointer shape names for OSC 22 (kitty 0.31+, ghostty, foot; terminals
// without the feature ignore the sequence). Names follow the CSS cursor
// values the spec builds on.
const (
	// ShapePointer marks spots that act on a click: toggles, buttons, rows.
	ShapePointer = "pointer"
	// ShapeText marks selectable or editable text.
	ShapeText = "text"
	// ShapeResizeEW marks horizontal drag handles, such as panel borders.
	ShapeResizeEW = "ew-resize"
)

// PointerShaper is implemented by widgets whose surface mixes interactive
// and passive regions. Coordinates are surface-local, as produced by
// Surface.HitTestAt. The empty string keeps the terminal's default pointer.
type PointerShaper interface {
	PointerShape(localX, localY int) string
}
