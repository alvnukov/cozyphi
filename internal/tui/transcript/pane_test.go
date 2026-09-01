package transcript

import (
	"fmt"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestTranscriptPane_ApplySessionAndSync(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "CozyPhi test")

	pane.ApplySession(session.UserAppend{Text: "hello"})
	pane.Sync()

	if pane.IsEmpty() {
		t.Fatal("expected transcript entries after user append")
	}
	if len(pane.Snapshot().Messages) != 1 {
		t.Fatalf("snap messages = %d, want 1", len(pane.Snapshot().Messages))
	}
}

func TestTranscriptPane_IsStreaming(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "CozyPhi test")

	if pane.IsStreaming() {
		t.Fatal("empty pane should not stream")
	}

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateStreaming,
	}})
	if !pane.IsStreaming() {
		t.Fatal("expected streaming after assistant StateStreaming")
	}

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateComplete,
	}})
	if pane.IsStreaming() {
		t.Fatal("expected idle after StreamEnd")
	}
}

func TestTranscriptPane_LoadReplayClearsWidgets(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	pane := NewTranscriptPane(th, spin, "CozyPhi test")

	pane.ApplySession(session.UserAppend{Text: "x"})
	pane.Sync()
	if pane.IsEmpty() {
		t.Fatal("setup: expected entries")
	}

	pane.LoadReplay(session.Snapshot{})
	pane.Sync()
	if !pane.IsEmpty() {
		t.Fatal("LoadReplay should clear visible entries until snap has items")
	}
}

func TestTranscriptPaneLoadReplayPublishesLatestUsage(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	var got session.TokenUsage
	pane.SetUsageCallback(func(usage session.TokenUsage) {
		got = usage
	})

	want := session.TokenUsage{PromptTokens: 1200, CompletionTokens: 80, TotalTokens: 1280}
	pane.LoadReplay(session.Snapshot{Messages: []session.Message{
		{ID: "a1", Role: session.RoleAssistant, State: session.StateComplete, Usage: want},
	}})

	if got != want {
		t.Fatalf("replayed usage = %+v, want %+v", got, want)
	}
}

func TestTranscriptPaneReplayPublishesCompactedContextAfterRetainedUsage(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	var got session.TokenUsage
	pane.SetUsageCallback(func(usage session.TokenUsage) {
		got = usage
	})

	want := session.TokenUsage{PromptTokens: 3200, TotalTokens: 3200, Estimated: true}
	pane.LoadReplay(session.Snapshot{Messages: []session.Message{
		{
			ID:    "retained-assistant",
			Role:  session.RoleAssistant,
			State: session.StateComplete,
			Usage: session.TokenUsage{PromptTokens: 12000, TotalTokens: 12100},
		},
		{ID: "compaction", Role: session.RoleCompaction, Usage: want},
	}})

	if got != want {
		t.Fatalf("replayed compacted usage = %+v, want %+v", got, want)
	}
}

func TestTranscriptPaneCompactionPublishesEstimatedContext(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	var got session.TokenUsage
	pane.SetUsageCallback(func(usage session.TokenUsage) {
		got = usage
	})

	want := session.TokenUsage{PromptTokens: 3200, TotalTokens: 3200, Estimated: true}
	pane.ApplySession(session.CompactionComplete{
		ID: "c1",
		Compaction: session.Compaction{
			TokensBefore:       12000,
			TokensAfter:        3200,
			MessagesSummarized: 6,
			MessagesKept:       2,
		},
	})

	if got != want {
		t.Fatalf("compacted usage = %+v, want %+v", got, want)
	}
}

func TestTranscriptPaneTailSyncUpdatesVisibleAssistant(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.ApplySession(streamingUpdate(0))
	pane.Sync()

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "assistant-current",
		State: session.StateStreaming,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "updated answer"},
		},
	}})
	pane.Sync()

	if len(pane.list.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(pane.list.Entries))
	}
	answer, ok := pane.list.Entries[0].(*block.AssistantBlock)
	if !ok || answer.Text != "updated answer" {
		t.Fatalf("assistant = %#v, want updated answer", pane.list.Entries[0])
	}
}

func TestTranscriptPaneTailSyncFallsBackWhenRowsChange(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.ApplySession(streamingUpdate(0))
	pane.Sync()

	pane.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "assistant-current",
		State: session.StateStreaming,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: "before tool"},
			{Type: session.BlockToolUse, ID: "tool-1", Name: "read", Input: "file.go"},
		},
	}})
	pane.Sync()

	if len(pane.list.Entries) != 2 {
		t.Fatalf("entries = %d, want assistant and tool rows", len(pane.list.Entries))
	}
	if _, ok := pane.list.Entries[1].(*block.ToolBlock); !ok {
		t.Fatalf("tail row = %T, want *block.ToolBlock", pane.list.Entries[1])
	}
}

// fixedRow is a pane-local fixed-height widget: exact scroll arithmetic
// without depending on block rendering.
type fixedRow struct {
	text string
	h    int
}

func (*fixedRow) Handle(*components.EventContext, xui.Event) {}

func (r *fixedRow) Draw(ctx components.DrawContext) components.Surface {
	s := components.NewSurface(max(ctx.Max.Width, 1), max(r.h, 1), r)
	s.Print(0, 0, r.text, xui.Style{}, ctx.Method)
	return s
}

func dragMouse(y int) xui.MouseEvent {
	return xui.MouseEvent{Action: xui.MouseDrag, Button: xui.MouseLeft, X: 2, Y: y}
}

// TestTranscriptPaneCleanClickFoldsAnExpandedBlock: a press+release with no
// drag between them folds the expanded block under the pointer, while a drag
// that selects text leaves the block open — selection wins over the click.
func TestTranscriptPaneCleanClickFoldsAnExpandedBlock(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	bash := &block.BashBlock{Command: "ls", Output: "one\ntwo\nthree", Expanded: true}
	pane.list.Entries = []components.Widget{bash}
	const listH = 12
	pane.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: listH}}, 40, listH)

	rowY := -1
	for y := listH - 1; y >= 0; y-- {
		if pane.list.IndexAtPoint(2, y) == 0 {
			rowY = y
			break
		}
	}
	if rowY < 0 {
		t.Fatal("expanded bash block not hit-testable")
	}

	ctx := &components.EventContext{}
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: rowY}, nil)
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MouseRelease, Button: xui.MouseLeft, X: 2, Y: rowY}, nil)
	if bash.Expanded {
		t.Fatal("clean click must fold the expanded block")
	}

	bash.Expanded = true
	pane.list.InvalidateHeights()
	pane.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: listH}}, 40, listH)
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: rowY}, nil)
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MouseDrag, Button: xui.MouseLeft, X: 10, Y: rowY}, nil)
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MouseRelease, Button: xui.MouseLeft, X: 10, Y: rowY}, nil)
	if !bash.Expanded {
		t.Fatal("a drag-selection must leave the block open")
	}
}

// TestTranscriptPaneSelectionEdgeAutoscroll: dragging a selection into the
// last rows keeps scrolling while the button is held — a slow crawl near the
// edge, faster on the edge row, faster still past the list into the composer
// zone; the selection endpoint rides the scroll; the top edge mirrors; ticks
// stop at the content bounds and on release.
func TestTranscriptPaneSelectionEdgeAutoscroll(t *testing.T) {
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	entries := make([]components.Widget, 40)
	for i := range entries {
		entries[i] = &fixedRow{text: fmt.Sprintf("row%02d", i), h: 1}
	}
	pane.list.Entries = entries
	const listH = 10
	pane.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: listH}}, 40, listH)
	// totalH = 1 + 40 + 39*1 = 80; maxScroll = 70.

	ctx := &components.EventContext{}
	pane.list.Handle(ctx, xui.MouseEvent{Button: xui.MouseWheelUp, Wheel: 10}) // sfb 30

	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 2, Y: 4}, nil)

	// Slow zone: three rows above the edge row drags nothing by itself;
	// the tick moves one row down and extends the selection endpoint.
	pane.HandleMouse(ctx, dragMouse(listH-3), nil)
	if pane.list.ScrollFromBottom != 30 {
		t.Fatalf("drag alone scrolled: sfb = %d, want 30", pane.list.ScrollFromBottom)
	}
	eyBefore := pane.sel.ey
	if !pane.AdvanceEdgeScroll() || pane.list.ScrollFromBottom != 29 {
		t.Fatalf("near-edge tick: sfb = %d, want 29", pane.list.ScrollFromBottom)
	}
	if pane.sel.ey != eyBefore+1 {
		t.Fatalf("selection endpoint rode %d rows, want +1", pane.sel.ey-eyBefore)
	}

	// Edge row: three rows per tick.
	pane.HandleMouse(ctx, dragMouse(listH-1), nil)
	if !pane.AdvanceEdgeScroll() || pane.list.ScrollFromBottom != 26 {
		t.Fatalf("edge-row tick: sfb = %d, want 26", pane.list.ScrollFromBottom)
	}

	// Past the list into the composer zone, three rows deep: 3 + 2*3 = 9.
	pane.HandleMouse(ctx, dragMouse(listH+2), nil)
	if !pane.AdvanceEdgeScroll() || pane.list.ScrollFromBottom != 17 {
		t.Fatalf("beyond-edge tick: sfb = %d, want 17", pane.list.ScrollFromBottom)
	}

	// Mid-view drag arms nothing.
	pane.HandleMouse(ctx, dragMouse(4), nil)
	if pane.AdvanceEdgeScroll() {
		t.Fatal("mid-view drag must not edge-scroll")
	}

	// Top edge row: three rows per tick toward history.
	pane.HandleMouse(ctx, dragMouse(0), nil)
	if !pane.AdvanceEdgeScroll() || pane.list.ScrollFromBottom != 20 {
		t.Fatalf("top-edge tick: sfb = %d, want 20", pane.list.ScrollFromBottom)
	}

	// Slow zone near the top: one row per tick.
	pane.HandleMouse(ctx, dragMouse(2), nil)
	if !pane.AdvanceEdgeScroll() || pane.list.ScrollFromBottom != 21 {
		t.Fatalf("near-top tick: sfb = %d, want 21", pane.list.ScrollFromBottom)
	}

	// Drive to the bottom at 9 rows/tick: 21 -> 12 -> 3 -> 0, then idle.
	pane.HandleMouse(ctx, dragMouse(listH+2), nil)
	for range 3 {
		if !pane.AdvanceEdgeScroll() {
			t.Fatal("edge scroll stopped before the bottom")
		}
	}
	if pane.list.ScrollFromBottom != 0 {
		t.Fatalf("exhaustion drive: sfb = %d, want 0", pane.list.ScrollFromBottom)
	}
	if pane.AdvanceEdgeScroll() {
		t.Fatal("at the bottom the tick must go idle")
	}

	// Release ends the drag and the auto-scroll.
	pane.HandleMouse(ctx, dragMouse(listH-1), nil)
	pane.HandleMouse(ctx, xui.MouseEvent{Action: xui.MouseRelease, Button: xui.MouseLeft, X: 2, Y: listH - 1}, nil)
	if pane.sel.dragging {
		t.Fatal("release must end the drag")
	}
	if pane.AdvanceEdgeScroll() {
		t.Fatal("release must stop edge scroll")
	}
}

// TestTranscriptPane_GluesConsecutiveToolRows: two tool-call rows render
// flush (no blank row between them), while a tool row and a non-tool row keep
// the single-row gap.
func TestTranscriptPane_GluesConsecutiveToolRows(t *testing.T) {
	th := components.DefaultTheme()
	pane := NewTranscriptPane(th, nil, "test")
	pane.list.Entries = []components.Widget{
		&block.ToolBlock{Name: "read", Status: status.ToolDone, Theme: th},
		&block.ToolBlock{Name: "grep", Status: status.ToolDone, Theme: th},
		&fixedRow{text: "after", h: 1},
	}
	s := pane.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 12}}, 40, 12)
	if len(s.Children) != 3 {
		t.Fatalf("children=%d, want 3", len(s.Children))
	}
	// Tool↔tool delta equals the first row's height (no blank row).
	got := s.Children[1].Origin.Y - s.Children[0].Origin.Y
	if got != s.Children[0].Surface.Size.Height {
		t.Fatalf("tool-tool delta=%d, want %d (flush)", got, s.Children[0].Surface.Size.Height)
	}
	// Tool↔non-tool delta is the row height plus one blank row (single gap).
	got = s.Children[2].Origin.Y - s.Children[1].Origin.Y
	if got != s.Children[1].Surface.Size.Height+1 {
		t.Fatalf("tool-row delta=%d, want %d (height+1)", got, s.Children[1].Surface.Size.Height+1)
	}
}
