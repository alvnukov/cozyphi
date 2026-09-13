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

// HoverTooltiper is implemented by widgets that can explain a hovered
// control in a line or two. Coordinates are surface-local, the same pair
// PointerShape received; the hint appears after the pointer dwells on the
// control, so it should say what a click would do, not describe the pixel.
// The boolean false (or empty text) keeps the dwell silent.
type HoverTooltiper interface {
	HoverTooltip(localX, localY int) (string, bool)
}

// PointerOwner is implemented by roots that can seize the whole pointer
// channel — a modal ask that eats every mouse event. While the root owns
// the pointer, the app holds its dwell hint back: the hit test would name
// a widget the panel covers.
type PointerOwner interface {
	OwnsPointer() bool
}
