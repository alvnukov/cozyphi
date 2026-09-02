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
}
