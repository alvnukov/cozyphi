package layout

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestTextDraw(t *testing.T) {
	txt := &Text{Content: "hi", Style: xui.Style{Bold: true}}
	s := txt.Draw(components.DrawContext{Max: components.Size{Width: 10, Height: 1}})
	if s.Size.Width < 2 || s.Size.Height != 1 {
		t.Fatalf("size = %+v", s.Size)
	}
	if s.Buffer[0].Char != "h" {
		t.Fatalf("cell0 = %+v", s.Buffer[0])
	}
}

func TestCenterAndHitTest(t *testing.T) {
	btn := &Button{Label: "Go"}
	c := &Center{Child: btn}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 5}})
	if len(s.Children) != 1 {
		t.Fatalf("children = %d", len(s.Children))
	}
	ch := s.Children[0]
	hit := s.HitTest(ch.Origin.X, ch.Origin.Y)
	if hit != btn {
		t.Fatalf("hit = %T", hit)
	}
}

func TestPaddingSizedBox(t *testing.T) {
	inner := &Text{Content: "hi"}
	p := &Padding{Insets: InsetsAll(1), Child: inner}
	s := p.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 10}})
	if s.Size.Width < 4 || s.Size.Height < 3 {
		t.Fatalf("padded size %+v", s.Size)
	}
	box := &SizedBox{Width: 10, Height: 5, Child: inner}
	bs := box.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}})
	if bs.Size.Width != 10 || bs.Size.Height != 5 {
		t.Fatalf("sizedbox %+v", bs.Size)
	}
}

func TestContainerBorder(t *testing.T) {
	c := Box(&Text{Content: "x"}, components.DefaultTheme().Border)
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 10}})
	if s.Buffer[0].Char != "╭" {
		t.Fatalf("expected rounded corner, got %q", s.Buffer[0].Char)
	}
}

func TestFlexibleColumn(t *testing.T) {
	col := &FlexColumn{
		Children: []components.Widget{
			&Text{Content: "top"},
			&Flexible{Flex: 1, Child: &Text{Content: "mid"}},
			&Text{Content: "bot"},
		},
	}
	s := col.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 10}})
	if len(s.Children) != 3 {
		t.Fatalf("children %d", len(s.Children))
	}
}
