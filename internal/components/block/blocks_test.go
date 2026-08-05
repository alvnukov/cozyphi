package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
)

func TestBashBlockRendersOutput(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "file.go")
	}
	b := &block.BashBlock{
		Command:  "ls",
		Output:   strings.Join(lines, "\n"),
		Status:   block.BashDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}})
	joined := components.SurfaceText(s)
	if !strings.Contains(joined, "$") || !strings.Contains(joined, "ls") {
		t.Fatalf("missing command: %q", joined)
	}
	if strings.Contains(joined, "Show more") || strings.Contains(joined, "lines truncated") {
		t.Fatalf("must not show Show-more chrome: %q", joined)
	}
	if !strings.Contains(joined, "file.go") {
		t.Fatalf("missing output: %q", joined)
	}
}

func TestUserAndAssistant(t *testing.T) {
	u := &block.UserBlock{Text: "hello", Theme: components.DefaultTheme()}
	us := u.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 5}})
	if !strings.Contains(components.SurfaceText(us), "$ hello") && !strings.Contains(components.SurfaceText(us), "hello") {
		t.Fatalf("user: %q", components.SurfaceText(us))
	}
	a := &block.AssistantBlock{Text: "see `xui` and examples/", Theme: components.DefaultTheme()}
	as := a.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 5}})
	txt := components.SurfaceText(as)
	if !strings.Contains(txt, "xui") || !strings.Contains(txt, "examples") {
		t.Fatalf("assistant: %q", txt)
	}
}

func TestStatusBlock(t *testing.T) {
	st := &block.StatusBlock{Label: "Thinking", Done: true, Expandable: true, Theme: components.DefaultTheme()}
	s := st.Draw(components.DrawContext{Max: components.Size{Width: 30, Height: 1}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "Thinking") || !strings.Contains(txt, "✓") {
		t.Fatalf("%q", txt)
	}
}

func TestUserBlockImplementsWidget(t *testing.T) {
	var _ components.Widget = &block.UserBlock{Text: "x", Theme: components.DefaultTheme()}
}
