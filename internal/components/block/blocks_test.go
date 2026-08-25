package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
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

	got := strings.TrimRight(components.SurfaceText(s), "\n")
	if !strings.Contains(got, report) {
		t.Fatalf("row = %q, want report %q", got, report)
	}
}

// TestCompactionBlockExpandable: a compaction with a summary toggles open on
// Enter or a click on the rule row — same contract as thinking blocks —
// showing the summarize body under the rule.
func TestCompactionBlockExpandable(t *testing.T) {
	report := "Compacted 12 messages · 56k → ~8k context · 4 kept"
	b := &block.CompactionBlock{Text: report, Summary: "slot module merged", Theme: components.DefaultTheme()}

	s := b.Draw(components.DrawContext{Max: components.Size{Width: 70, Height: 10}, Method: xui.WidthUnicode})
	if s.Size.Height != 1 || !strings.Contains(components.SurfaceText(s), "▶") {
		t.Fatalf("collapsed row = %q (height %d)", components.SurfaceText(s), s.Size.Height)
	}

	ctx := &components.EventContext{}
	b.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter})
	if !b.Expanded || !ctx.Consume {
		t.Fatalf("expanded=%v consume=%v after Enter", b.Expanded, ctx.Consume)
	}

	s = b.Draw(components.DrawContext{Max: components.Size{Width: 70, Height: 10}, Method: xui.WidthUnicode})
	got := components.SurfaceText(s)
	if s.Size.Height < 2 || !strings.Contains(got, "slot module merged") || !strings.Contains(got, "▼") {
		t.Fatalf("expanded row = %q (height %d)", got, s.Size.Height)
	}

	// Clicking the body must not toggle; clicking the rule row must.
	b.Handle(&components.EventContext{}, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, Y: 1})
	if !b.Expanded {
		t.Fatal("body click toggled expansion")
	}
	b.Handle(&components.EventContext{}, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, Y: 0})
	if b.Expanded {
		t.Fatal("rule click did not collapse")
	}
}

// TestCompactionBlockSummaryRendersMarkdown: the summary body uses the shared
// Markdown renderer, stripping markers and keeping themed inline styles.
func TestCompactionBlockSummaryRendersMarkdown(t *testing.T) {
	th := components.DefaultTheme()
	b := &block.CompactionBlock{
		Text:     "Compacted",
		Summary:  "merged **slot** module",
		Expanded: true,
		Theme:    th,
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 70, Height: 10}, Method: xui.WidthUnicode})

	got := components.SurfaceText(s)
	if !strings.Contains(got, "merged slot module") {
		t.Fatalf("summary = %q, want markdown text with markers stripped", got)
	}
	if strings.Contains(got, "**") {
		t.Fatalf("summary = %q, still contains markdown markers", got)
	}
	if !th.Foreground.Equal(s.Buffer[s.Size.Width+3].Style) {
		t.Fatalf("summary lead style = %+v, want foreground", s.Buffer[s.Size.Width+3].Style)
	}

	var sawStrong bool
	for y := 1; y < s.Size.Height; y++ {
		for x := 0; x < s.Size.Width; x++ {
			sawStrong = sawStrong || s.Buffer[y*s.Size.Width+x].Style.Equal(th.Markdown.Strong)
		}
	}
	if !sawStrong {
		t.Fatal("summary body has no strong-styled span")
	}
}

// TestCompactionBlockWithoutSummaryStaysInert: legacy rows without a summary
// keep the plain rule — no affordance, no toggling.
func TestCompactionBlockWithoutSummaryStaysInert(t *testing.T) {
	b := &block.CompactionBlock{Text: "Compacted", Theme: components.DefaultTheme()}
	ctx := &components.EventContext{}
	b.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter})
	if b.Expanded || ctx.Consume {
		t.Fatalf("expanded=%v consume=%v; inert row must not toggle", b.Expanded, ctx.Consume)
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 70, Height: 10}, Method: xui.WidthUnicode})
	if strings.Contains(components.SurfaceText(s), "▶") {
		t.Fatal("inert row shows an affordance arrow")
	}
}

// TestBlocksPointerShapes pins the hover pointer per surface region: the hand
// appears exactly where a left click acts, everything else in the transcript
// is a text beam.
func TestBlocksPointerShapes(t *testing.T) {
	ctx := components.DrawContext{Max: components.Size{Width: 60, Height: 20}, Method: xui.WidthUnicode}
	check := func(name string, w components.PointerShaper, wantTitle, wantBody string, surfaceH int) {
		t.Helper()
		if got := w.PointerShape(0, 0); got != wantTitle {
			t.Fatalf("%s title shape = %q, want %q", name, got, wantTitle)
		}
		if surfaceH > 1 {
			if got := w.PointerShape(0, surfaceH-1); got != wantBody {
				t.Fatalf("%s body shape = %q, want %q", name, got, wantBody)
			}
		}
	}

	tool := &block.ToolBlock{Name: "read", Output: "data", Expanded: true, Theme: components.DefaultTheme()}
	check("tool", tool, components.ShapePointer, components.ShapeText, tool.Draw(ctx).Size.Height)
	inertTool := &block.ToolBlock{Name: "read", Theme: components.DefaultTheme()}
	check("bodyless tool", inertTool, components.ShapeText, "", inertTool.Draw(ctx).Size.Height)

	agent := &block.AgentBlock{
		Name:     "agent",
		Children: []block.ChildTool{{Name: "read", Detail: "a.go", Status: status.ToolDone}},
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	check("agent", agent, components.ShapePointer, components.ShapeText, agent.Draw(ctx).Size.Height)

	bash := &block.BashBlock{
		Command:  "ls",
		Output:   "out",
		Status:   block.BashDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	check("bash", bash, components.ShapePointer, components.ShapeText, bash.Draw(ctx).Size.Height)

	comp := &block.CompactionBlock{
		Text:     "compacted",
		Summary:  "kept tokens",
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	check("compaction", comp, components.ShapePointer, components.ShapeText, comp.Draw(ctx).Size.Height)

	think := &block.ThinkingBlock{Text: "hmm", Expanded: true, Theme: components.DefaultTheme()}
	check("thinking", think, components.ShapePointer, components.ShapeText, think.Draw(ctx).Size.Height)

	expandableStatus := &block.StatusBlock{Label: "run", Expandable: true, Theme: components.DefaultTheme()}
	check(
		"expandable status",
		expandableStatus,
		components.ShapePointer,
		components.ShapePointer,
		expandableStatus.Draw(ctx).Size.Height,
	)
	plainStatus := &block.StatusBlock{Label: "run", Theme: components.DefaultTheme()}
	check("plain status", plainStatus, components.ShapeText, components.ShapeText, plainStatus.Draw(ctx).Size.Height)

	user := &block.UserBlock{Text: "hi", Theme: components.DefaultTheme()}
	check("user", user, components.ShapeText, components.ShapeText, user.Draw(ctx).Size.Height)
	assistant := &block.AssistantBlock{Text: "yo", Theme: components.DefaultTheme()}
	check("assistant", assistant, components.ShapeText, components.ShapeText, assistant.Draw(ctx).Size.Height)
}
