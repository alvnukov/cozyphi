package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

func modelControlInput() *ChatInput {
	th := components.DefaultTheme()
	return &ChatInput{
		MinBodyRows: 1,
		Theme:       th,
		AgentLabel:  layout.BorderLabel{Text: "⏵⏵ build", Style: th.Secondary},
		ModelName:   "gpt-5",
		EffortLabel: "high",
		Value:       "hello",
		Cursor:      5,
	}
}

func TestChatInputModelControlsRenderAndReserveEffort(t *testing.T) {
	c := modelControlInput()
	c.OnModelPick = func(components.Point) {}
	c.OnEffortPick = func(components.Point) {}

	s := c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
	})
	meta := rowString(s, 3)
	if !strings.Contains(meta, "⏵⏵ build · gpt-5 ▾ · high ▾") {
		t.Fatalf("meta row = %q", meta)
	}
	if c.modelHit.x1 <= c.modelHit.x0 || c.effortHit.x1 <= c.effortHit.x0 {
		t.Fatalf("missing control hits: model=%+v effort=%+v", c.modelHit, c.effortHit)
	}
	if got := c.PointerShape(c.modelHit.x0, c.modelHit.y); got != components.ShapePointer {
		t.Fatalf("model pointer shape = %q", got)
	}
	if got := c.PointerShape(c.effortHit.x0, c.effortHit.y); got != components.ShapePointer {
		t.Fatalf("effort pointer shape = %q", got)
	}

	narrow := c.Draw(components.DrawContext{
		Max:    components.Size{Width: 32, Height: 12},
		Method: xui.WidthUnicode,
	})
	meta = rowString(narrow, 3)
	if !strings.Contains(meta, "high ▾") || !strings.Contains(meta, c.EditingLabel()) {
		t.Fatalf("narrow meta must retain effort and editing mode: %q", meta)
	}
}

func TestChatInputModelControlHoverRegions(t *testing.T) {
	c := modelControlInput()
	c.OnModelPick = func(components.Point) {}
	c.OnEffortPick = func(components.Point) {}
	_ = c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
	})

	if got := c.HoverRegion(c.modelHit.x0, c.modelHit.y); got != 1 {
		t.Fatalf("model hover region = %d, want 1", got)
	}
	if got := c.HoverRegion(c.modelHit.x1-1, c.modelHit.y); got != 1 {
		t.Fatalf("model tail hover region = %d, want 1", got)
	}
	if got := c.HoverRegion(c.effortHit.x0, c.effortHit.y); got != 2 {
		t.Fatalf("effort hover region = %d, want 2", got)
	}
	if got := c.HoverRegion(c.modelHit.x1, c.modelHit.y); got != 0 {
		t.Fatalf("separator hover region = %d, want 0", got)
	}

	c.OnModelPick, c.OnEffortPick = nil, nil
	if got := c.HoverRegion(c.modelHit.x0, c.modelHit.y); got != 0 {
		t.Fatalf("callback-less model hover region = %d, want 0", got)
	}
	if got := c.HoverRegion(c.effortHit.x0, c.effortHit.y); got != 0 {
		t.Fatalf("callback-less effort hover region = %d, want 0", got)
	}
}

func TestChatInputModelControlClicksDoNotMoveCaretOrSelection(t *testing.T) {
	c := modelControlInput()
	modelPicks, effortPicks := 0, 0
	var modelAt, effortAt components.Point
	c.OnModelPick = func(at components.Point) {
		modelPicks++
		modelAt = at
	}
	c.OnEffortPick = func(at components.Point) {
		effortPicks++
		effortAt = at
	}
	c.SetSelection(1, 4)
	_ = c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
	})
	cursor, selected := c.Cursor, c.SelectedText()

	ctx := &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{
		Action: xui.MousePress,
		Button: xui.MouseLeft,
		X:      c.modelHit.x0 + 2,
		Y:      c.modelHit.y,
	})
	if modelPicks != 1 || effortPicks != 0 || !ctx.Consume || !ctx.Redraw {
		t.Fatalf("model click: model=%d effort=%d ctx=%+v", modelPicks, effortPicks, ctx)
	}
	if modelAt != (components.Point{X: c.modelHit.x0 + 2, Y: c.modelHit.y}) {
		t.Fatalf("model callback lost local click: %v", modelAt)
	}
	if c.Cursor != cursor || c.SelectedText() != selected {
		t.Fatalf("model click changed edit state: cursor=%d selection=%q", c.Cursor, c.SelectedText())
	}

	ctx = &components.EventContext{}
	c.Handle(ctx, xui.MouseEvent{
		Action: xui.MousePress,
		Button: xui.MouseLeft,
		X:      c.effortHit.x0 + 1,
		Y:      c.effortHit.y,
	})
	if modelPicks != 1 || effortPicks != 1 || !ctx.Consume {
		t.Fatalf("effort click: model=%d effort=%d ctx=%+v", modelPicks, effortPicks, ctx)
	}
	if effortAt != (components.Point{X: c.effortHit.x0 + 1, Y: c.effortHit.y}) {
		t.Fatalf("effort callback lost local click: %v", effortAt)
	}
	if c.Cursor != cursor || c.SelectedText() != selected {
		t.Fatalf("effort click changed edit state: cursor=%d selection=%q", c.Cursor, c.SelectedText())
	}
}

func TestChatInputModelControlHoverIsClippedAndSearchClearsHits(t *testing.T) {
	c := modelControlInput()
	c.OnModelPick = func(components.Point) {}
	c.OnEffortPick = func(components.Point) {}
	base := c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
	})
	hit := c.modelHit
	baseControl := base.Buffer[hit.y*base.Size.Width+hit.x0].Style
	baseBefore := base.Buffer[hit.y*base.Size.Width+hit.x0-1].Style

	hovered := c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
		Hover:  &components.HoverState{Widget: c, X: hit.x0, Y: hit.y},
	})
	if got := hovered.Buffer[hit.y*hovered.Size.Width+hit.x0].Style; got.Equal(baseControl) {
		t.Fatal("model control did not highlight on hover")
	}
	if got := hovered.Buffer[hit.y*hovered.Size.Width+hit.x0-1].Style; !got.Equal(baseBefore) {
		t.Fatal("hover escaped the clipped model hit rectangle")
	}

	c.search.active = true
	_ = c.Draw(components.DrawContext{
		Max:    components.Size{Width: 60, Height: 12},
		Method: xui.WidthUnicode,
	})
	if c.modelHit != (metaHit{}) || c.effortHit != (metaHit{}) {
		t.Fatalf("search retained stale hits: model=%+v effort=%+v", c.modelHit, c.effortHit)
	}
	if got := c.PointerShape(hit.x0, hit.y); got != components.ShapeText {
		t.Fatalf("stale search pointer shape = %q", got)
	}
}
