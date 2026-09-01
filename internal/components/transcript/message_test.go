package transcript

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// rowStub is a fixed-height Widget used to exercise MessageList without
// importing the block subpackage (avoids components↔block test-type cycles).
type rowStub struct {
	text string
	h    int
}

type cachedRowStub struct {
	surface components.Surface
}

func (*cachedRowStub) Handle(_ *components.EventContext, _ xui.Event) {}

func (r *cachedRowStub) Draw(components.DrawContext) components.Surface { return r.surface }

func (*rowStub) Handle(_ *components.EventContext, _ xui.Event) {}

func (r *rowStub) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	w = max(w, 1)
	h := r.h
	h = max(h, 1)
	s := components.NewSurface(w, h, r)
	s.Print(0, 0, r.text, xui.Style{}, ctx.Method)
	return s
}

// TestMessageListSidePadding: the transcript insets entries two columns on
// each side, like opencode's paddingLeft/Right=2 message container.
func TestMessageListSidePadding(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{&rowStub{text: "one", h: 1}},
	}
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 4}})
	if len(s.Children) == 0 {
		t.Fatal("expected visible children")
	}
	if got := s.Children[0].Origin.X; got != 1 {
		t.Fatalf("entry starts at x=%d, want 1 (opencode side padding)", got)
	}
	if got := s.Children[0].Surface.Size.Width; got != 38 {
		t.Fatalf("entry width=%d, want 38 (40 minus 1 padding per side)", got)
	}
}

func TestMessageListBottomPin(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{
			&rowStub{text: "one", h: 1},
			&rowStub{text: "two", h: 1},
			&rowStub{text: "three", h: 1},
		},
	}
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 4}})
	if len(s.Children) == 0 {
		t.Fatal("expected visible children")
	}
	last := s.Children[len(s.Children)-1]
	if last.Origin.Y+last.Surface.Size.Height > 4 {
		t.Fatalf("last overflows: origin=%+v h=%d", last.Origin, last.Surface.Size.Height)
	}
}

func TestMessageListInvalidateHeightsAt(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{
			&rowStub{text: "a", h: 2},
			&rowStub{text: "b", h: 3},
			&rowStub{text: "c", h: 4},
		},
	}
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	if list.CachedHeight(0) != 2 || list.CachedHeight(1) != 3 || list.CachedHeight(2) != 4 {
		t.Fatalf("heights %d %d %d", list.CachedHeight(0), list.CachedHeight(1), list.CachedHeight(2))
	}
	list.InvalidateHeightsAt(1)
	if list.CachedHeight(0) != 2 || list.CachedHeight(1) != 0 || list.CachedHeight(2) != 4 {
		t.Fatalf("after invalidate: %d %d %d", list.CachedHeight(0), list.CachedHeight(1), list.CachedHeight(2))
	}
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	if list.CachedHeight(1) != 3 {
		t.Fatalf("remeasured h=%d", list.CachedHeight(1))
	}
}

func TestMessageListReindexHeights(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{
			&rowStub{text: "a", h: 2},
			&rowStub{text: "b", h: 5},
			&rowStub{text: "c", h: 3},
		},
	}
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	oldIDs := []string{"a", "b", "c"}
	// Insert "x" between a and b; heights must follow ids, not old indices.
	list.Entries = []components.Widget{
		&rowStub{text: "a", h: 2},
		&rowStub{text: "x", h: 7},
		&rowStub{text: "b", h: 5},
		&rowStub{text: "c", h: 3},
	}
	list.ReindexHeights(oldIDs, []string{"a", "x", "b", "c"})
	if list.CachedHeight(0) != 2 || list.CachedHeight(1) != 0 || list.CachedHeight(2) != 5 ||
		list.CachedHeight(3) != 3 {
		t.Fatalf(
			"reindex: %d %d %d %d",
			list.CachedHeight(0),
			list.CachedHeight(1),
			list.CachedHeight(2),
			list.CachedHeight(3),
		)
	}
	list.InvalidateHeightsAt(1)
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	if list.CachedHeight(1) != 7 {
		t.Fatalf("new row h=%d", list.CachedHeight(1))
	}
}

func TestMessageListVirtualizes(t *testing.T) {
	const n = 80
	entries := make([]components.Widget, n)
	for i := range n {
		entries[i] = &rowStub{text: "row", h: 1}
	}
	list := &MessageList{Entries: entries}
	const viewH = 6
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: viewH}})
	if len(s.Children) >= n {
		t.Fatalf("expected windowed draw, children=%d for %d entries", len(s.Children), n)
	}
	if len(s.Children) > viewH+2 {
		t.Fatalf("too many realized children: %d (viewH=%d)", len(s.Children), viewH)
	}
	first, last := list.VisibleRange()
	if first < 0 || last < first {
		t.Fatalf("visible range %d..%d", first, last)
	}
	if last != n-1 {
		t.Fatalf("bottom pin: last visible=%d want %d", last, n-1)
	}
	list.ScrollFromBottom = 40
	s2 := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: viewH}})
	f2, l2 := list.VisibleRange()
	if len(s2.Children) == 0 || l2 >= n-1 && f2 == first {
		t.Fatalf("scroll did not move window: %d..%d (was %d..%d)", f2, l2, first, last)
	}
}

// TestMessageListDefaultSpacing: entries separate by one blank row by
// default, matching opencode's marginTop.
func TestMessageListDefaultSpacing(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{
			&rowStub{text: "one", h: 1},
			&rowStub{text: "two", h: 1},
		},
	}
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	if len(s.Children) != 2 {
		t.Fatalf("children=%d, want 2", len(s.Children))
	}
	delta := s.Children[1].Origin.Y - s.Children[0].Origin.Y
	if delta != 2 { // 1 content row + 1 spacing
		t.Fatalf("row delta=%d, want 2 (one row + one blank)", delta)
	}
}

func TestMessageListGapBetweenGluesRows(t *testing.T) {
	list := &MessageList{
		Entries: []components.Widget{
			&rowStub{text: "a", h: 2},
			&rowStub{text: "b", h: 1},
		},
		GapBetween: func(_, _ components.Widget) int {
			return 0 // glue every pair
		},
	}
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 20}})
	if len(s.Children) != 2 {
		t.Fatalf("children=%d, want 2", len(s.Children))
	}
	// delta = first row height + 0 glue.
	if got := s.Children[1].Origin.Y - s.Children[0].Origin.Y; got != 2 {
		t.Fatalf("glued delta=%d, want 2 (row h=2, no gap)", got)
	}
}

// TestMessageListTopSpacer: content starts one row below the top of the
// scroll extent, opencode's leading <box height={1}/> — so the first message
// never glues to the top edge when scrolled home.
func TestMessageListTopSpacer(t *testing.T) {
	entries := make([]components.Widget, 20)
	for i := range entries {
		entries[i] = &rowStub{text: "row", h: 1}
	}
	list := &MessageList{Entries: entries}
	list.ScrollFromBottom = 1 << 20 // force scroll home; Draw clamps
	s := list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 6}})
	if len(s.Children) == 0 {
		t.Fatal("expected visible children")
	}
	if got := s.Children[0].Origin.Y; got != 1 {
		t.Fatalf("first row at y=%d, want 1 (spacer row above content)", got)
	}
}

func TestMessageListSelectionDoesNotMutateCachedSurface(t *testing.T) {
	row := &cachedRowStub{surface: components.NewSurface(20, 1, nil)}
	row.surface.Print(0, 0, "row", xui.Style{}, xui.WidthUnicode)
	list := &MessageList{Entries: []components.Widget{row}, Selected: 0, Theme: components.DefaultTheme()}
	ctx := components.DrawContext{Max: components.Size{Width: 24, Height: 3}, Method: xui.WidthUnicode}

	selected := list.Draw(ctx)
	if selected.Children[0].Surface.Buffer[0].Style.Bg.Kind == 0 {
		t.Fatal("selected row was not highlighted")
	}
	if row.surface.Buffer[0].Style.Bg.Kind != 0 {
		t.Fatal("selection mutated widget-owned cached surface")
	}

	list.Selected = -1
	unselected := list.Draw(ctx)
	if unselected.Children[0].Surface.Buffer[0].Style.Bg.Kind != 0 {
		t.Fatal("selection highlight leaked into later frame")
	}
}

// TestMessageListScrollByClamps: ScrollBy is the one clamped scroll seam —
// positive rows follow the tail, negative rows reach into history, both stop
// at the content extent, and the signed return reports what actually moved.
func TestMessageListScrollByClamps(t *testing.T) {
	entries := make([]components.Widget, 10)
	for i := range entries {
		entries[i] = &rowStub{text: "row", h: 1}
	}
	list := &MessageList{Entries: entries}
	ctx := components.DrawContext{Max: components.Size{Width: 40, Height: 6}}
	_ = list.Draw(ctx) // totalH = 1 + 10 + 9*1 = 20; maxScroll = 14

	if got := list.ScrollBy(-5); got != -5 || list.ScrollFromBottom != 5 {
		t.Fatalf("ScrollBy(-5) = %d, sfb = %d; want -5, 5", got, list.ScrollFromBottom)
	}
	if got := list.ScrollBy(-100); got != -9 || list.ScrollFromBottom != 14 {
		t.Fatalf("ScrollBy(-100) = %d, sfb = %d; want -9, 14 (clamp home)", got, list.ScrollFromBottom)
	}
	if got := list.ScrollBy(100); got != 14 || list.ScrollFromBottom != 0 {
		t.Fatalf("ScrollBy(100) = %d, sfb = %d; want 14, 0 (clamp bottom)", got, list.ScrollFromBottom)
	}
	if got := list.ScrollBy(4); got != 0 || list.ScrollFromBottom != 0 {
		t.Fatalf("ScrollBy(4) at bottom = %d, sfb = %d; want 0, 0", got, list.ScrollFromBottom)
	}
}

// TestMessageListGrowthAnchorsScrolledView: while the reader is scrolled up,
// growth below the viewport (streaming tail, appended rows) must not shove
// what they are reading — ScrollFromBottom absorbs the delta and the top
// visible row stays put. Follow mode (0) keeps following the tail.
func TestMessageListGrowthAnchorsScrolledView(t *testing.T) {
	entries := make([]components.Widget, 20)
	for i := range entries {
		entries[i] = &rowStub{text: "row", h: 1}
	}
	list := &MessageList{Entries: entries}
	ctx := components.DrawContext{Max: components.Size{Width: 40, Height: 6}}
	_ = list.Draw(ctx) // totalH = 1 + 20 + 19*1 = 40

	list.ScrollFromBottom = 20
	_ = list.Draw(ctx)
	topBefore, _ := list.VisibleRange()

	list.Entries = append(list.Entries, &rowStub{text: "tail", h: 1})
	_ = list.Draw(ctx) // totalH grows by 1 gap row + 1 row = 2

	if got := list.ScrollFromBottom; got != 22 {
		t.Fatalf("ScrollFromBottom = %d, want 22 (growth absorbed by anchor)", got)
	}
	topAfter, _ := list.VisibleRange()
	if topAfter != topBefore {
		t.Fatalf("top visible row moved: %d -> %d", topBefore, topAfter)
	}

	list.StickToBottom()
	_ = list.Draw(ctx)
	if list.ScrollFromBottom != 0 {
		t.Fatalf("follow mode lost: sfb = %d", list.ScrollFromBottom)
	}
	_, last := list.VisibleRange()
	if last != 20 {
		t.Fatalf("follow mode shows last entry %d, want 20", last)
	}
}

// TestMessageListPointerShapeText: the transcript surface is selectable text;
// interactive rows (block title rows) override this from their own surfaces.
func TestMessageListPointerShapeText(t *testing.T) {
	list := &MessageList{Entries: []components.Widget{&rowStub{text: "one", h: 1}}}
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 4}})
	if got := list.PointerShape(2, 2); got != components.ShapeText {
		t.Fatalf("list shape = %q, want text", got)
	}
}

// TestMessageListPageKeepsOneRowOfOverlap: page keys move a screen minus one
// row — the TUI-wide dialect — so the reader keeps their footing at the seam.
func TestMessageListPageKeepsOneRowOfOverlap(t *testing.T) {
	entries := make([]components.Widget, 40)
	for i := range entries {
		entries[i] = &rowStub{text: "row", h: 1}
	}
	list := &MessageList{Entries: entries}
	_ = list.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})

	ctx := &components.EventContext{}
	list.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyPageUp})
	if !ctx.Consume {
		t.Fatal("PageUp must be consumed")
	}
	if got := list.ScrollFromBottom; got != 9 {
		t.Fatalf("ScrollFromBottom=%d after PageUp, want 9 (a screen minus one overlap row)", got)
	}
	list.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyPageDown})
	if got := list.ScrollFromBottom; got != 0 {
		t.Fatalf("ScrollFromBottom=%d after PageDown, want 0", got)
	}
}
