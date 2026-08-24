package transcript

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
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
	if got := s.Children[0].Origin.X; got != 2 {
		t.Fatalf("entry starts at x=%d, want 2 (opencode side padding)", got)
	}
	if got := s.Children[0].Surface.Size.Width; got != 36 {
		t.Fatalf("entry width=%d, want 36 (40 minus 2 padding per side)", got)
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

func TestMessageListSelectionDoesNotMutateCachedSurface(t *testing.T) {
	row := &cachedRowStub{surface: components.NewSurface(20, 1, nil)}
	row.surface.Print(0, 0, "row", xui.Style{}, xui.WidthUnicode)
	list := &MessageList{Entries: []components.Widget{row}, Selected: 0}
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
