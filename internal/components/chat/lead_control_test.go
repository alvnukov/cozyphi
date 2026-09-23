package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

const leadHint = "Ask a side question — answer stays out of context (Ctrl+T)"

func leadControlInput() *ChatInput {
	c := modelControlInput()
	c.SessionLabel = "main"
	return c
}

func drawLead(c *ChatInput, width int) components.Surface {
	return c.Draw(components.DrawContext{Max: components.Size{Width: width, Height: 12}, Method: xui.WidthUnicode})
}

func leadCell(t *testing.T, s components.Surface) int {
	t.Helper()
	row := rowString(s, 3)
	before, _, ok := strings.Cut(row, "⏵⏵ build")
	if !ok {
		t.Fatalf("lead missing from %q", row)
	}
	return xui.StringWidth(before, xui.WidthUnicode)
}

func TestChatInputLeadClickPreservesEditorAndOtherControls(t *testing.T) {
	c := leadControlInput()
	leadClicks, modelClicks, effortClicks := 0, 0, 0
	c.OnLeadClick = func() { leadClicks++ }
	c.OnModelPick = func(components.Point) { modelClicks++ }
	c.OnEffortPick = func(components.Point) { effortClicks++ }
	c.SetSelection(1, 4)
	s := drawLead(c, 70)
	if s.Size.Height != 6 || !strings.Contains(rowString(s, 3), "main · ⏵⏵ build · gpt-5 ▾ · high ▾") {
		t.Fatalf("lead changed composer layout: height=%d row=%q", s.Size.Height, rowString(s, 3))
	}
	leadX := leadCell(t, s)
	cursor, selected := c.Cursor, c.SelectedText()
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: leadX + 1, Y: 3})
	if leadClicks != 1 || modelClicks != 0 || effortClicks != 0 || !ctx.Consume || !ctx.Redraw {
		t.Fatalf("lead click: lead=%d model=%d effort=%d ctx=%+v", leadClicks, modelClicks, effortClicks, ctx)
	}
	if c.Cursor != cursor || c.SelectedText() != selected {
		t.Fatalf("lead click changed edit state: cursor=%d selection=%q", c.Cursor, c.SelectedText())
	}
	for _, ev := range []xui.MouseEvent{
		{Action: xui.MouseRelease, Button: xui.MouseLeft, X: leadX + 1, Y: 3},
		{Action: xui.MousePress, Button: xui.MouseRight, X: leadX + 1, Y: 3},
		{Action: xui.MouseMotion, X: leadX + 1, Y: 3},
		{Action: xui.MousePress, Button: xui.MouseLeft, X: leadX - 1, Y: 3},
		{Action: xui.MousePress, Button: xui.MouseLeft, X: c.modelHit.x0, Y: 3},
		{Action: xui.MousePress, Button: xui.MouseLeft, X: c.effortHit.x0, Y: 3},
	} {
		c.Handle(&components.EventContext{}, ev)
	}
	if leadClicks != 1 || modelClicks != 1 || effortClicks != 1 {
		t.Fatalf("press/release or neighbor routing: lead=%d model=%d effort=%d", leadClicks, modelClicks, effortClicks)
	}
}

func TestChatInputLeadHoverHintAndTintOnlyPaintedCells(t *testing.T) {
	c := leadControlInput()
	c.OnLeadClick = func() {}
	c.LeadTooltip = func() string { return leadHint }
	base := drawLead(c, 70)
	x := leadCell(t, base)
	end := x + xui.StringWidth("⏵⏵ build", xui.WidthUnicode)
	for _, at := range []int{x, end - 1} {
		if c.HoverRegion(at, 3) != 3 || c.PointerShape(at, 3) != components.ShapePointer {
			t.Fatalf("lead cell %d not clickable", at)
		}
		if hint, ok := c.HoverTooltip(at, 3); !ok || hint != leadHint {
			t.Fatalf("lead hint at %d = %q, %v", at, hint, ok)
		}
	}
	for _, at := range []int{x - 1, end, 0} {
		if c.HoverRegion(at, 3) != 0 || c.PointerShape(at, 3) != components.ShapeText {
			t.Fatalf("neighbor cell %d clickable", at)
		}
		if hint, ok := c.HoverTooltip(at, 3); ok || hint != "" {
			t.Fatalf("neighbor hint at %d = %q, %v", at, hint, ok)
		}
	}
	if _, ok := c.HoverTooltip(x, 2); ok {
		t.Fatal("hint escaped meta row")
	}
	hover := c.Draw(components.DrawContext{
		Max: components.Size{Width: 70, Height: 12}, Method: xui.WidthUnicode,
		Hover: &components.HoverState{Widget: c, X: x, Y: 3},
	})
	if hover.Buffer[3*70+x].Style.Equal(base.Buffer[3*70+x].Style) {
		t.Fatal("lead did not tint on hover")
	}
	for _, at := range []int{x - 1, end} {
		if !hover.Buffer[3*70+at].Style.Equal(base.Buffer[3*70+at].Style) {
			t.Fatalf("hover tint escaped lead at %d", at)
		}
	}
}

func TestChatInputLeadPassiveWithoutCallback(t *testing.T) {
	c := leadControlInput()
	s := drawLead(c, 70)
	x := leadCell(t, s)
	if c.HoverRegion(x, 3) != 0 || c.PointerShape(x, 3) != components.ShapeText {
		t.Fatal("passive lead became interactive")
	}
	if hint, ok := c.HoverTooltip(x, 3); ok || hint != "" {
		t.Fatalf("passive lead hint = %q, %v", hint, ok)
	}
	hover := c.Draw(components.DrawContext{
		Max: components.Size{Width: 70, Height: 12}, Method: xui.WidthUnicode,
		Hover: &components.HoverState{Widget: c, X: x, Y: 3},
	})
	if !hover.Buffer[3*70+x].Style.Equal(s.Buffer[3*70+x].Style) {
		t.Fatal("passive lead tinted on hover")
	}
}

func TestChatInputLeadHitClearsOnClippedAndSearchDraw(t *testing.T) {
	c := leadControlInput()
	clicks := 0
	c.OnLeadClick = func() { clicks++ }
	wide := drawLead(c, 70)
	x := leadCell(t, wide)
	if c.HoverRegion(x, 3) != 3 {
		t.Fatal("wide lead not clickable")
	}
	// Narrow width reserves the effort control and mode badge ahead of the lead.
	narrow := drawLead(c, 22)
	if strings.Contains(rowString(narrow, 3), "⏵⏵") {
		t.Fatalf("test needs a fully clipped lead: %q", rowString(narrow, 3))
	}
	if c.HoverRegion(x, 3) != 0 {
		t.Fatal("clipped draw retained lead hit")
	}
	if _, ok := c.HoverTooltip(x, 3); ok {
		t.Fatal("clipped draw retained hint")
	}
	c.Handle(&components.EventContext{}, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: x, Y: 3})
	if clicks != 0 {
		t.Fatal("clipped lead fired")
	}
	drawLead(c, 70)
	c.search.active = true
	drawLead(c, 70)
	if c.HoverRegion(x, 3) != 0 || c.PointerShape(x, 3) != components.ShapeText {
		t.Fatal("search retained lead hit")
	}
	if _, ok := c.HoverTooltip(x, 3); ok {
		t.Fatal("search retained lead hint")
	}
}
