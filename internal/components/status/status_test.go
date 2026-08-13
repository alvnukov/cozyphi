package status

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
)

func TestExpandable(t *testing.T) {
	ex := &Expandable{
		Title:      &layout.Text{Content: "Title"},
		Child:      &layout.Text{Content: "Body"},
		Expandable: true,
		Expanded:   true,
		Theme:      components.DefaultTheme(),
	}
	s := ex.Draw(components.DrawContext{Max: components.Size{Width: 30, Height: 10}})
	if s.Size.Height < 2 {
		t.Fatalf("expandable height %d", s.Size.Height)
	}
}

func TestToolHeaderSpinner(t *testing.T) {
	sp := NewSpinner(xui.Style{})
	sp.Tick()
	h := &ToolHeader{Name: "bash", Detail: "ls", Status: ToolRunning, Spinner: sp, Theme: components.DefaultTheme()}
	s := h.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 1}})
	if s.Size.Height < 1 {
		t.Fatal("empty tool header")
	}
}
