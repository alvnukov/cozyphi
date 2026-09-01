package block

import (
	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// messageIndent is the left inset of assistant-side transcript entries,
// matching opencode's paddingLeft=3 on message parts: the role gutter bar in
// column 0, a gap, then the content. Bodies of expandable blocks sit two
// columns deeper. User messages are full-width panels and take no inset.
const messageIndent = 3

// gutterGlyph is the role gutter bar. It is deliberately not the tree/table
// "│": selection copy filters it out as chrome, and box-drawing content must
// survive that filter.
const gutterGlyph = "▏"

// gutterBar paints the role gutter down every row of a block surface, after
// the content is painted. The bar's color is the block's one role/status
// signal: quiet for working rows, muted for the assistant's own text,
// destructive when the row carries a failure.
func gutterBar(s *components.Surface, st xui.Style) {
	w := s.Size.Width
	if w <= 0 || s.Buffer == nil {
		return
	}
	for y := range s.Size.Height {
		s.Buffer[y*w] = xui.Cell{Char: gutterGlyph, Width: 1, Style: st}
	}
}

// quietGutter is the gutter style of working rows (thinking, tools, folds):
// present enough to read the turn's shape, dim enough to stay out of the way.
func quietGutter(th components.Theme) xui.Style {
	st := th.Muted
	st.Dim = true
	return st
}
