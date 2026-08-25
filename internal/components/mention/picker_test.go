package mention

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestPickerAccept(t *testing.T) {
	var got string
	p := &Picker{
		Items: []Item{{Path: "go.mod"}, {Path: "a/b.go"}},
		OnAccept: func(item Item) {
			got = item.Path
		},
	}
	p.Show()
	p.Selected = 1
	if !p.Accept() {
		t.Fatal("accept failed")
	}
	if got != "a/b.go" {
		t.Fatalf("got %q", got)
	}
	if p.Open {
		t.Fatal("should be closed")
	}
}

func TestPickerHandleNav(t *testing.T) {
	p := &Picker{
		Items: []Item{{Path: "a"}, {Path: "b"}, {Path: "c"}},
	}
	p.Show()
	if !p.HandleNav(xui.KeyEvent{Press: true, Code: xui.KeyDown}) {
		t.Fatal("expected consume")
	}
	if p.Selected != 1 {
		t.Fatalf("selected=%d", p.Selected)
	}
	if !p.HandleNav(xui.KeyEvent{Press: true, Code: xui.KeyEscape}) {
		t.Fatal("expected consume")
	}
	if p.Open {
		t.Fatal("should close on escape")
	}
}

func TestPickerDrawClosed(t *testing.T) {
	p := &Picker{Theme: components.DefaultTheme()}
	surf := p.Draw(components.DrawContext{
		Max:    components.Size{Width: 80, Height: 24},
		Method: xui.WidthUnicode,
	})
	if len(surf.Children) != 0 {
		t.Fatal("closed picker should have no children")
	}
}

func TestPickerDrawOpen(t *testing.T) {
	p := &Picker{
		Theme:         components.DefaultTheme(),
		Items:         []Item{{Path: "go.mod"}, {Path: "internal/x.go"}},
		AnchorBottomY: 20,
		AnchorWidth:   60,
		AnchorX:       0,
	}
	p.Show()
	surf := p.Draw(components.DrawContext{
		Max:    components.Size{Width: 80, Height: 24},
		Method: xui.WidthUnicode,
	})
	if len(surf.Children) != 1 {
		t.Fatalf("children=%d", len(surf.Children))
	}
	child := surf.Children[0]
	if child.Origin.Y+child.Surface.Size.Height > 20 {
		t.Fatalf("panel should sit above anchor: oy=%d h=%d", child.Origin.Y, child.Surface.Size.Height)
	}
}
