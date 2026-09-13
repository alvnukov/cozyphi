package block_test

import (
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
)

// hoverTint reports the background of the first non-empty cell of row y.
func hoverTint(t *testing.T, s components.Surface, y int) xui.Color {
	t.Helper()
	if y >= s.Size.Height {
		t.Fatalf("row %d beyond surface height %d", y, s.Size.Height)
	}
	// Skip the gutter bar column: it repaints its cell after the hover
	// tint and legitimately carries no background.
	for x := 2; x < s.Size.Width; x++ {
		c := s.Buffer[y*s.Size.Width+x]
		if c.Char != "" {
			return c.Style.Bg
		}
	}
	return xui.Color{}
}

func hoveredCtx(w components.Widget) components.DrawContext {
	return components.DrawContext{
		Max:   components.Size{Width: 40, Height: 20},
		Hover: &components.HoverState{Widget: w, X: 1, Y: 0},
	}
}

// The affordance mirrors the pointer gates: the hand shows on the title row
// of a block with a body — the hover tint paints exactly those rows, and
// only when the hover names this widget.
func TestHoverTintsInteractiveRows(t *testing.T) {
	th := components.DefaultTheme()
	wantBg := th.BackgroundElement.Bg

	tool := &block.ToolBlock{Name: "read", Output: "body", Expanded: true, Theme: th}
	s := tool.Draw(hoveredCtx(tool))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("tool title row bg = %v, want %v", got, wantBg)
	}
	if got := hoverTint(t, s, 1); got == wantBg {
		t.Fatal("tool body row tinted outside the interactive region")
	}

	// A hover naming another widget must not tint this one.
	s = tool.Draw(components.DrawContext{
		Max:   components.Size{Width: 40, Height: 20},
		Hover: &components.HoverState{Widget: &block.ToolBlock{}, X: 1, Y: 0},
	})
	if got := hoverTint(t, s, 0); got == wantBg {
		t.Fatal("tool title tinted for a hover on another widget")
	}
}

func TestHoverGatesMirrorClickability(t *testing.T) {
	th := components.DefaultTheme()
	wantBg := th.BackgroundElement.Bg

	// Tool row without a body: no toggle, no hand, no tint.
	bare := &block.ToolBlock{Name: "read", Theme: th}
	s := bare.Draw(hoveredCtx(bare))
	if got := hoverTint(t, s, 0); got == wantBg {
		t.Fatal("bodyless tool row tinted")
	}

	bash := &block.BashBlock{Command: "ls", Output: "out", Theme: th}
	s = bash.Draw(hoveredCtx(bash))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("bash title row bg = %v, want %v", got, wantBg)
	}

	agent := &block.AgentBlock{Summary: "summary text", Theme: th}
	s = agent.Draw(hoveredCtx(agent))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("agent title row bg = %v, want %v", got, wantBg)
	}

	comp := &block.CompactionBlock{Text: "compacted", Summary: "sum", Theme: th}
	s = comp.Draw(hoveredCtx(comp))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("compaction title row bg = %v, want %v", got, wantBg)
	}

	think := &block.ThinkingBlock{Text: "reasoning", Theme: th}
	s = think.Draw(hoveredCtx(think))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("thinking title row bg = %v, want %v", got, wantBg)
	}

	sb := &block.StatusBlock{Label: "turn", Expandable: true, Theme: th}
	s = sb.Draw(hoveredCtx(sb))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("expandable status row bg = %v, want %v", got, wantBg)
	}
	inert := &block.StatusBlock{Label: "turn", Theme: th}
	s = inert.Draw(hoveredCtx(inert))
	if got := hoverTint(t, s, 0); got == wantBg {
		t.Fatal("inert status row tinted")
	}

	// The two rows that offer the hand everywhere they fold: the diff title
	// (only with a body, mirroring its click gate) and the turn-summary row
	// (always — the whole row is the fold handle).
	diff := &block.DiffBlock{Name: "edit", Path: "a.go", Diff: "+x", Theme: th}
	s = diff.Draw(hoveredCtx(diff))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("diff title row bg = %v, want %v", got, wantBg)
	}
	bareDiff := &block.DiffBlock{Name: "edit", Path: "a.go", Theme: th}
	s = bareDiff.Draw(hoveredCtx(bareDiff))
	if got := hoverTint(t, s, 0); got == wantBg {
		t.Fatal("bodyless diff row tinted")
	}

	turn := &block.TurnSummaryBlock{Rows: 3, Theme: th}
	s = turn.Draw(hoveredCtx(turn))
	if got := hoverTint(t, s, 0); got != wantBg {
		t.Fatalf("turn-summary row bg = %v, want %v", got, wantBg)
	}
}

// The hint names the fold state of the row under the pointer — the same
// rows the shape gates: the title row of a block with a body, never the
// body itself, never a bodyless title.
func TestBlockHintsFollowTheFold(t *testing.T) {
	th := components.DefaultTheme()

	collapsed := &block.ToolBlock{Name: "read", Output: "body", Theme: th}
	collapsed.Draw(hoveredCtx(collapsed)) // Draw measures the title row the gate checks.
	if got, ok := collapsed.HoverTooltip(1, 0); !ok || got != "unfold — show the tool call's detail" {
		t.Fatalf("collapsed tool hint = %q, %v", got, ok)
	}
	expanded := &block.ToolBlock{Name: "read", Output: "body", Expanded: true, Theme: th}
	expanded.Draw(hoveredCtx(expanded))
	if got, ok := expanded.HoverTooltip(1, 0); !ok || got != "fold — hide the tool call's detail" {
		t.Fatalf("expanded tool hint = %q, %v", got, ok)
	}
	if _, ok := expanded.HoverTooltip(1, 1); ok {
		t.Fatal("tool body row hinted")
	}
	bare := &block.ToolBlock{Name: "read", Theme: th}
	if _, ok := bare.HoverTooltip(1, 0); ok {
		t.Fatal("bodyless tool row hinted")
	}

	turn := &block.TurnSummaryBlock{Rows: 3, Theme: th}
	if got, ok := turn.HoverTooltip(1, 0); !ok || got != "unfold — show this turn's messages" {
		t.Fatalf("turn hint = %q, %v", got, ok)
	}
	turn.Expanded = true
	if got, ok := turn.HoverTooltip(1, 0); !ok || got != "fold — hide this turn's messages" {
		t.Fatalf("expanded turn hint = %q, %v", got, ok)
	}
}
