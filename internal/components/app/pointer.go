package app

import (
	"github.com/alvnukov/cozyphi/internal/components"
)

// pointerShapeSeq builds the OSC 22 sequence that sets the mouse pointer
// shape; an empty shape resets the pointer to the terminal default.
func pointerShapeSeq(shape string) string {
	return "\x1b]22;" + shape + "\x1b\\"
}

// pointerShapeAt resolves the pointer shape for a screen cell by asking the
// deepest widget under it. Widgets without interior structure — and misses —
// keep the terminal default.
func pointerShapeAt(surf components.Surface, x, y int) string {
	w, lx, ly := surf.HitTestAt(x, y)
	if shaper, ok := w.(components.PointerShaper); ok {
		return shaper.PointerShape(lx, ly)
	}
	return ""
}

// updatePointerShape emits OSC 22 whenever the shape under the pointer
// changed. The shape is computed against the last painted frame — exactly
// what the user sees under the pointer — and deduped so motion over passive
// areas costs nothing.
func (a *App) updatePointerShape(x, y int) {
	shape := pointerShapeAt(a.lastSurf, x, y)
	if shape == a.pointerShape {
		return
	}
	a.pointerShape = shape
	_, _ = a.vx.WriteRaw([]byte(pointerShapeSeq(shape)))
}
