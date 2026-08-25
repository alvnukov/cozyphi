package input

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

func TestDiffBlock(t *testing.T) {
	d := &DiffBlock{Diff: "+added\n-removed\n context", Theme: components.DefaultTheme()}
	ds := d.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})
	if ds.Size.Height != 3 {
		t.Fatalf("diff lines %d", ds.Size.Height)
	}
}

func TestModalMarkdown(t *testing.T) {
	md := &Markdown{Source: "# Hello\n- item `code`", Theme: components.DefaultTheme()}
	ms := md.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})
	if ms.Size.Height < 2 {
		t.Fatalf("markdown h=%d", ms.Size.Height)
	}
	modal := &Modal{
		Title:  "Confirm",
		Body:   &layout.Text{Content: "Sure?"},
		Footer: "Esc close",
		Width:  40,
		Theme:  components.DefaultTheme(),
	}
	s := modal.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}})
	if len(s.Children) != 1 {
		t.Fatalf("modal children %d", len(s.Children))
	}
}
