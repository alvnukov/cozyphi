package browse

// Ring is the selection of a short fixed choice list — a modal's options.
// Unlike Cursor it wraps: a four-row modal has no scroll, and a key that
// stops dead at an edge reads as a key that does nothing.
type Ring struct {
	selected int
	n        int
}

// SetLen tells the ring how many options exist, keeping the selection
// inside the new list. Call it whenever the options are rebuilt — a
// filtered list shrinks and grows under the selection.
func (r *Ring) SetLen(n int) {
	r.n = max(n, 0)
	r.selected = min(r.selected, max(r.n-1, 0))
}

// Selected is the current option; zero when the ring is empty.
func (r *Ring) Selected() int { return r.selected }

// Select puts the selection on option i, clamped into the list.
func (r *Ring) Select(i int) { r.selected = min(max(i, 0), max(r.n-1, 0)) }

// Step moves delta options, wrapping at both edges.
func (r *Ring) Step(delta int) {
	if r.n <= 0 {
		return
	}
	r.selected = ((r.selected+delta)%r.n + r.n) % r.n
}
