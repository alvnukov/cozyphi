package transcript

import (
	"slices"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// MessageList is a bottom-anchored scrollable transcript with windowed
// (virtualized) drawing: only entries that intersect the viewport are Draw()'n
// each frame. Off-screen heights are cached so scroll extent stays correct.
type MessageList struct {
	Entries []components.Widget
	// ScrollFromBottom is how many rows above the natural bottom pin.
	// 0 means stick to bottom (follow mode).
	ScrollFromBottom int
	PaddingX         int
	ItemSpacing      int // blank rows between entries; default 1
	// GapBetween, when set, returns the blank rows between two adjacent
	// entries (prev, next). A negative result falls back to ItemSpacing; the
	// transcript uses it to glue consecutive tool-call rows (0 rows).
	GapBetween func(prev, next components.Widget) int
	// TopPad is the blank row(s) above the content, opencode's leading
	// spacer; default 1.
	TopPad int
	Theme  components.Theme
	// Selected is the highlighted entry index for block copy; -1 = none.
	Selected int

	// totalH is last measured content height (for scroll clamping).
	totalH int
	viewH  int

	// height cache (virtualization)
	heights []int
	cacheW  int

	// last layout for hit-testing (viewport-visible items, list-local Y).
	lastPad    int
	lastOrigin int
	lastItems  []listItemGeom
}

type listItemGeom struct {
	index int
	y, h  int // list-local (screen within the list surface)
}

func (m *MessageList) spacing() int {
	if m.ItemSpacing <= 0 {
		return 1
	}
	return m.ItemSpacing
}

// topPad is the blank row above the first entry, opencode's leading
// <box height={1}/> spacer, so scrolled-home content never glues to the top.
func (m *MessageList) topPad() int {
	if m.TopPad < 0 {
		return 0
	}
	if m.TopPad == 0 {
		return 1
	}
	return m.TopPad
}

func (m *MessageList) padX() int {
	if m.PaddingX < 0 {
		return 0
	}
	if m.PaddingX == 0 {
		// opencode's message container insets one column per side.
		return 1
	}
	return m.PaddingX
}

// InvalidateHeights drops cached row heights (call after an entry's size changes
// in a way Draw won't see until remount — e.g. external expand). Visible rows
// remasure every frame already.
func (m *MessageList) InvalidateHeights() {
	m.heights = nil
	m.cacheW = 0
}

// InvalidateHeight marks a single row for remasure on the next Draw.
func (m *MessageList) InvalidateHeight(i int) {
	if i >= 0 && i < len(m.heights) {
		m.heights[i] = 0
	}
}

// InvalidateHeightsAt marks the given row indices for remasure.
func (m *MessageList) InvalidateHeightsAt(indices ...int) {
	for _, i := range indices {
		m.InvalidateHeight(i)
	}
}

// ReindexHeights rebuilds the height cache so each newIDs[i] keeps the height
// previously cached for the same id. Unknown ids stay 0 (remeasured next Draw).
// Call before InvalidateHeightsAt when the entry id sequence may have shifted.
func (m *MessageList) ReindexHeights(oldIDs, newIDs []string) {
	if len(m.heights) == 0 || m.cacheW == 0 {
		return
	}
	byID := make(map[string]int, len(oldIDs))
	for i, id := range oldIDs {
		if id == "" || i >= len(m.heights) {
			continue
		}
		if h := m.heights[i]; h > 0 {
			byID[id] = h
		}
	}
	next := make([]int, len(newIDs))
	for i, id := range newIDs {
		if h, ok := byID[id]; ok {
			next[i] = h
		}
	}
	m.heights = next
}

// CachedHeight returns the cached height for index i, or 0 if missing/invalidated.
func (m *MessageList) CachedHeight(i int) int {
	if i < 0 || i >= len(m.heights) {
		return 0
	}
	return m.heights[i]
}

func (m *MessageList) syncHeightCache(n, innerW int) {
	if m.cacheW != innerW {
		m.heights = make([]int, n)
		m.cacheW = innerW
		return
	}
	if len(m.heights) < n {
		m.heights = append(m.heights, make([]int, n-len(m.heights))...)
	} else if len(m.heights) > n {
		m.heights = m.heights[:n]
	}
}

func (m *MessageList) measure(i int, childCtx components.DrawContext) int {
	if i < 0 || i >= len(m.Entries) || m.Entries[i] == nil {
		return 1
	}
	surf := m.Entries[i].Draw(childCtx)
	h := surf.Size.Height
	h = max(h, 1)
	return h
}

// gapBefore returns the blank rows to insert before entry i, using GapBetween
// for the (i-1, i) pair when it returns a non-negative value, and falling back
// to spacing() otherwise.
func (m *MessageList) gapBefore(i int) int {
	if i <= 0 || i > len(m.Entries) {
		return 0
	}
	if m.GapBetween != nil {
		if g := m.GapBetween(m.Entries[i-1], m.Entries[i]); g >= 0 {
			return g
		}
	}
	return m.spacing()
}

func (m *MessageList) contentOffsets(n int) (tops []int, total int) {
	tops = make([]int, n)
	y := m.topPad()
	for i := range n {
		if i > 0 {
			y += m.gapBefore(i)
		}
		tops[i] = y
		h := 1
		if i < len(m.heights) && m.heights[i] > 0 {
			h = m.heights[i]
		}
		y += h
	}
	return tops, y
}

// Handle scrolls on PageUp/PageDown and mouse wheel, and forwards events
// to visible entries (last drawn first).
func (m *MessageList) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		switch e.Code {
		// A page keeps one row of overlap so the reader can see the seam —
		// the TUI-wide dialect (internal/tui/browse sits above this package,
		// so the rule is restated here, not imported).
		case xui.KeyPageUp:
			m.ScrollBy(-max(m.viewH-1, 1))
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyPageDown:
			m.ScrollBy(max(m.viewH-1, 1))
			ctx.ConsumeAndRedraw()
			return
		}
	case xui.MouseEvent:
		wheel := e.Wheel
		wheel = max(wheel, 1)
		if e.Button == xui.MouseWheelUp {
			m.ScrollBy(-3 * wheel)
			ctx.ConsumeAndRedraw()
			return
		}
		if e.Button == xui.MouseWheelDown {
			m.ScrollBy(3 * wheel)
			ctx.ConsumeAndRedraw()
			return
		}
	}
	// Prefer last-visible entries; fall back to all if layout not ready.
	if len(m.lastItems) > 0 {
		for i := range slices.Backward(m.lastItems) {
			idx := m.lastItems[i].index
			if idx < 0 || idx >= len(m.Entries) {
				continue
			}
			m.Entries[idx].Handle(ctx, ev)
			if ctx.Consume {
				return
			}
		}
		return
	}
	for i := range slices.Backward(m.Entries) {
		m.Entries[i].Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
}

// Draw renders the bottom-anchored windowed entries, measuring heights on
// demand and clamping ScrollFromBottom to the content extent.
func (m *MessageList) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	h := ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 10
	}
	m.viewH = h
	pad := m.padX()
	innerW := w - pad*2
	innerW = max(innerW, 1)
	n := len(m.Entries)
	childCtx := ctx.WithConstraints(components.Size{}, components.Size{Width: innerW, Height: 10000})

	m.syncHeightCache(n, innerW)

	// Ensure every row has a height (measure missing only — O(new/invalidated)).
	for i := range n {
		if m.heights[i] < 1 {
			m.heights[i] = m.measure(i, childCtx)
		}
	}

	var root components.Surface
	prevTotal := m.totalH
	for pass := range 2 {
		tops, totalH := m.contentOffsets(n)
		if pass == 0 && m.ScrollFromBottom > 0 && prevTotal > 0 && totalH > prevTotal {
			// The reader scrolled up: anchor the top visible row so growth
			// below the viewport (streaming tail, appended rows) widens the
			// scroll extent instead of shoving their view downward.
			m.ScrollFromBottom += totalH - prevTotal
		}
		m.totalH = totalH
		maxScroll := m.totalH - h
		maxScroll = max(maxScroll, 0)
		if m.ScrollFromBottom > maxScroll {
			m.ScrollFromBottom = maxScroll
		}
		if m.ScrollFromBottom < 0 {
			m.ScrollFromBottom = 0
		}
		originY := h - m.totalH + m.ScrollFromBottom
		m.lastPad = pad
		m.lastOrigin = originY

		root = components.Surface{Size: components.Size{Width: w, Height: h}, Widget: m}
		m.lastItems = m.lastItems[:0]
		heightChanged := false

		for i := range n {
			itemH := m.heights[i]
			itemH = max(itemH, 1)
			top := originY + tops[i]
			bot := top + itemH
			if bot <= 0 || top >= h {
				continue // outside viewport — skip Draw
			}
			surf := m.Entries[i].Draw(childCtx)
			nh := surf.Size.Height
			nh = max(nh, 1)
			if nh != m.heights[i] {
				m.heights[i] = nh
				heightChanged = true
			}
			if i == m.Selected && m.Selected >= 0 {
				surf = components.CloneSurface(surf)
				components.ApplyBlockHighlight(&surf, m.Theme.BlockHighlight)
			}
			root.Children = append(root.Children, components.SubSurface{
				Origin:  components.Point{X: pad, Y: top},
				Surface: surf,
			})
			m.lastItems = append(m.lastItems, listItemGeom{index: i, y: top, h: nh})
		}

		if !heightChanged || pass == 1 {
			break
		}
		// Visible row height changed (expand/collapse) — relayout once.
	}
	return root
}

// ScrollBy moves the viewport by rows: positive rows follow the tail (newer
// content), negative rows reach into history; both stop at the content
// extent from the last Draw. The signed return is the rows actually moved,
// so callers riding the scroll (drag selection) can track what slid under
// the pointer.
func (m *MessageList) ScrollBy(rows int) int {
	if rows == 0 {
		return 0
	}
	maxScroll := max(m.totalH-m.viewH, 0)
	next := min(max(m.ScrollFromBottom-rows, 0), maxScroll)
	moved := m.ScrollFromBottom - next
	m.ScrollFromBottom = next
	return moved
}

// PointerShape marks the transcript surface as selectable text; interactive
// rows (block title rows) override this from their own surfaces.
func (*MessageList) PointerShape(_, _ int) string { return components.ShapeText }

// StickToBottom resets follow mode.
func (m *MessageList) StickToBottom() {
	m.ScrollFromBottom = 0
}

// TopEntryIndex returns the index of the entry on the viewport's top row in
// the last Draw's layout, or -1 before any layout.
func (m *MessageList) TopEntryIndex() int {
	if len(m.lastItems) == 0 {
		return -1
	}
	return m.lastItems[0].index
}

// ScrollToEntry scrolls so entry i's first row sits on the viewport's top
// row, clamped to the content extent. It reports whether the viewport moved.
func (m *MessageList) ScrollToEntry(i int) bool {
	n := len(m.Entries)
	if i < 0 || i >= n || m.viewH <= 0 {
		return false
	}
	tops, total := m.contentOffsets(n)
	maxScroll := max(total-m.viewH, 0)
	target := min(max(total-m.viewH-tops[i], 0), maxScroll)
	moved := target != m.ScrollFromBottom
	m.ScrollFromBottom = target
	return moved
}

// ContentOrigin is the list-local Y of content row 0 after the last Draw.
// Viewport Y → content Y: y - ContentOrigin(); reverse: y + ContentOrigin().
func (m *MessageList) ContentOrigin() int {
	return m.lastOrigin
}

// IndexAtPoint returns the entry index under list-local (x,y), or -1.
func (m *MessageList) IndexAtPoint(x, y int) int {
	_ = x
	for _, it := range m.lastItems {
		if y >= it.y && y < it.y+it.h {
			return it.index
		}
	}
	return -1
}

// SelectedCopyText returns the copyable text of the selected entry.
func (m *MessageList) SelectedCopyText() string {
	if m.Selected < 0 || m.Selected >= len(m.Entries) {
		return ""
	}
	return components.EntryCopyText(m.Entries[m.Selected])
}

// LastCopyText returns copyable text from the last entry that supports it.
func (m *MessageList) LastCopyText() string {
	for i := range slices.Backward(m.Entries) {
		if t := components.EntryCopyText(m.Entries[i]); t != "" {
			return t
		}
	}
	return ""
}

// VisibleRange returns the inclusive [first, last] entry indices realized in
// the last Draw, or (-1, -1) if none.
func (m *MessageList) VisibleRange() (first, last int) {
	if len(m.lastItems) == 0 {
		return -1, -1
	}
	return m.lastItems[0].index, m.lastItems[len(m.lastItems)-1].index
}
