package status_test

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

// tintOf reads a row's tint from the parent buffer, descending into child
// surfaces that cover the row when the parent cells are empty (the
// Expandable title paints into a child).
func tintOf(s components.Surface, y int) xui.Color {
	for x := 0; x < s.Size.Width; x++ {
		c := s.Buffer[y*s.Size.Width+x]
		if c.Char != "" {
			return c.Style.Bg
		}
	}
	for _, ch := range s.Children {
		ly := y - ch.Origin.Y
		if ly < 0 || ly >= ch.Surface.Size.Height {
			continue
		}
		for x := 0; x < ch.Surface.Size.Width; x++ {
			c := ch.Surface.Buffer[ly*ch.Surface.Size.Width+x]
			if c.Char != "" {
				return c.Style.Bg
			}
		}
	}
	return xui.Color{}
}

// The hover affordance mirrors PointerShape: the title while expanded, the
// whole collapsed block, never when not expandable.
func TestExpandableHoverTint(t *testing.T) {
	th := components.DefaultTheme()
	wantBg := th.BackgroundElement.Bg

	open := &status.Expandable{
		Title:      &layout.Text{Content: "Title"},
		Child:      &layout.Text{Content: "Body"},
		Expandable: true,
		Expanded:   true,
		Theme:      th,
	}
	s := open.Draw(components.DrawContext{
		Max:   components.Size{Width: 30, Height: 10},
		Hover: &components.HoverState{Widget: open, X: 1, Y: 0},
	})
	if s.Size.Height < 2 {
		t.Fatalf("open expandable height %d", s.Size.Height)
	}
	if tintOf(s, 0) != wantBg {
		t.Fatalf("open title row bg = %v, want %v", tintOf(s, 0), wantBg)
	}
	if tintOf(s, 1) == wantBg {
		t.Fatal("open body row tinted")
	}

	collapsed := &status.Expandable{
		Title:      &layout.Text{Content: "Title"},
		Expandable: true,
		Theme:      th,
	}
	s = collapsed.Draw(components.DrawContext{
		Max:   components.Size{Width: 30, Height: 10},
		Hover: &components.HoverState{Widget: collapsed, X: 1, Y: 0},
	})
	if tintOf(s, 0) != wantBg {
		t.Fatal("collapsed expandable title not tinted")
	}

	inert := &status.Expandable{Title: &layout.Text{Content: "T"}, Theme: th}
	s = inert.Draw(components.DrawContext{
		Max:   components.Size{Width: 30, Height: 10},
		Hover: &components.HoverState{Widget: inert, X: 1, Y: 0},
	})
	if tintOf(s, 0) == wantBg {
		t.Fatal("non-expandable title tinted")
	}
}

func TestListTileHoverTint(t *testing.T) {
	th := components.DefaultTheme()
	wantBg := th.BackgroundElement.Bg

	tappable := &status.ListTile{Title: "row", OnTap: func() {}, Theme: th}
	s := tappable.Draw(components.DrawContext{
		Max:   components.Size{Width: 30, Height: 10},
		Hover: &components.HoverState{Widget: tappable, X: 1, Y: 0},
	})
	if tintOf(s, 0) != wantBg {
		t.Fatal("tappable tile not tinted")
	}

	plain := &status.ListTile{Title: "row", Theme: th}
	s = plain.Draw(components.DrawContext{
		Max:   components.Size{Width: 30, Height: 10},
		Hover: &components.HoverState{Widget: plain, X: 1, Y: 0},
	})
	if tintOf(s, 0) == wantBg {
		t.Fatal("passive tile tinted")
	}
}
