package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/components/status"
)

func TestBashBlockRendersOutput(t *testing.T) {
	var lines []string
	for range 20 {
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
	if !strings.Contains(components.SurfaceText(us), "$ hello") &&
		!strings.Contains(components.SurfaceText(us), "hello") {
		t.Fatalf("user: %q", components.SurfaceText(us))
	}
	a := &block.AssistantBlock{Text: "see `xui` and examples/", Theme: components.DefaultTheme()}
	as := a.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 5}})
	txt := components.SurfaceText(as)
	if !strings.Contains(txt, "xui") || !strings.Contains(txt, "examples") {
		t.Fatalf("assistant: %q", txt)
	}
}

func TestAgentBlockRendersTreeAndMarkdown(t *testing.T) {
	a := &block.AgentBlock{
		Name:   "agent_spawn",
		Detail: "find bug",
		Status: status.ToolDone,
		Children: []block.ChildTool{
			{Name: "read", Detail: "a.go", Status: status.ToolDone},
			{Name: "bash", Detail: "go test", Status: status.ToolError},
		},
		Summary:  "## Findings\n\n- fixed",
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	s := a.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 40}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "agent_spawn") || !strings.Contains(txt, "find bug") {
		t.Fatalf("title: %q", txt)
	}
	if !strings.Contains(txt, "├──") || !strings.Contains(txt, "╰──") {
		t.Fatalf("missing tree connectors: %q", txt)
	}
	if !strings.Contains(txt, "read") || !strings.Contains(txt, "bash") {
		t.Fatalf("missing children: %q", txt)
	}
	if !strings.Contains(txt, "Findings") || !strings.Contains(txt, "fixed") {
		t.Fatalf("missing markdown summary: %q", txt)
	}
	if strings.Contains(txt, `"job_id"`) || strings.Contains(txt, `"summary"`) {
		t.Fatalf("must not show raw JSON: %q", txt)
	}
}

func TestUserBlockImplementsWidget(_ *testing.T) {
	var _ components.Widget = &block.UserBlock{Text: "x", Theme: components.DefaultTheme()}
}

// TestAssistantFamilyInset: every assistant-side block opens three columns in
// (opencode paddingLeft=3 on message parts), so titles and text line up in a
// shared left rail.
func TestAssistantFamilyInset(t *testing.T) {
	blocks := map[string]components.Widget{
		"assistant": &block.AssistantBlock{Text: "hello", Theme: components.DefaultTheme()},
		"thinking":  &block.ThinkingBlock{Text: "hmm", Theme: components.DefaultTheme()},
		"tool":      &block.ToolBlock{Name: "read", Detail: "a.go", Theme: components.DefaultTheme()},
		"bash":      &block.BashBlock{Command: "ls", Theme: components.DefaultTheme()},
		"agent":     &block.AgentBlock{Name: "agent_spawn", Theme: components.DefaultTheme()},
	}
	for name, w := range blocks {
		s := w.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})
		firstX := -1
		for x := 0; x < s.Size.Width && firstX == -1; x++ {
			c := s.Buffer[x]
			if c.Char != "" && c.Char != " " {
				firstX = x
			}
		}
		if firstX != 3 {
			t.Errorf("%s first content at x=%d, want 3", name, firstX)
		}
	}
}

// TestCompactionBlockCenteredRule: compaction renders as opencode's centered
// " Compaction " top rule in the border color, one row high.
func TestCompactionBlockCenteredRule(t *testing.T) {
	th := components.DefaultTheme()
	b := &block.CompactionBlock{Theme: th}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 5}, Method: xui.WidthUnicode})
	if s.Size.Height != 1 {
		t.Fatalf("height=%d, want 1", s.Size.Height)
	}
	got := strings.TrimRight(components.SurfaceText(s), "\n")
	if want := "──── Compaction ────"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	if !th.Border.Equal(s.Buffer[0].Style) {
		t.Fatalf("rule style = %+v, want border color", s.Buffer[0].Style)
	}
}

func TestCompactionBlockShowsReport(t *testing.T) {
	report := "Compacted 12 messages · 56k → ~8k context · 4 kept"
	b := &block.CompactionBlock{Text: report, Theme: components.DefaultTheme()}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 70, Height: 5}, Method: xui.WidthUnicode})

	got := strings.TrimRight(components.SurfaceText(s), "\\n")
	if !strings.Contains(got, report) {
		t.Fatalf("row = %q, want report %q", got, report)
	}
}
