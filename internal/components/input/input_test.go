package input

import (
	"strings"
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

func TestTextFieldCappedViewportFollowsWrappedCursor(t *testing.T) {
	field := &TextField{Value: "aaaaabbbbbcccccdddd", Cursor: len("aaaaabbbbbcccccdddd"), MaxLines: 2}

	surface := field.Draw(components.DrawContext{Max: components.Size{Width: 5, Height: 2}})

	if got := surfaceRow(surface, 0); got != "ccccc" {
		t.Fatalf("first visible row = %q, want %q", got, "ccccc")
	}
	if got := surfaceRow(surface, 1); got != "dddd" {
		t.Fatalf("second visible row = %q, want %q", got, "dddd")
	}
	if surface.Cursor == nil || *surface.Cursor != (components.Point{X: 4, Y: 1}) {
		t.Fatalf("cursor = %+v, want x=4 y=1", surface.Cursor)
	}
}

func TestTextFieldCappedViewportReturnsToWrappedCursorNearStart(t *testing.T) {
	field := &TextField{Value: "aaaaabbbbbcccccdddd", Cursor: 2, MaxLines: 2}

	surface := field.Draw(components.DrawContext{Max: components.Size{Width: 5, Height: 2}})

	if got := surfaceRow(surface, 0); got != "aaaaa" {
		t.Fatalf("first visible row = %q, want %q", got, "aaaaa")
	}
	if got := surfaceRow(surface, 1); got != "bbbbb" {
		t.Fatalf("second visible row = %q, want %q", got, "bbbbb")
	}
	if surface.Cursor == nil || *surface.Cursor != (components.Point{X: 2, Y: 0}) {
		t.Fatalf("cursor = %+v, want x=2 y=0", surface.Cursor)
	}
}

func surfaceRow(surface components.Surface, row int) string {
	var b strings.Builder
	for x := 0; x < surface.Size.Width; x++ {
		b.WriteString(surface.Buffer[row*surface.Size.Width+x].Char)
	}
	return strings.TrimRight(b.String(), " ")
}
