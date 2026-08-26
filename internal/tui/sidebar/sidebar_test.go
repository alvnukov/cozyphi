package sidebar

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/session"
)

func ctrlO(upper bool) xui.KeyEvent {
	r := rune('o')
	if upper {
		r = 'O'
	}
	return xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r, Mods: xui.ModCtrl}
}

func drawText(s *Sidebar, height int) string {
	return components.SurfaceText(s.Draw(components.DrawContext{
		Max:    components.Size{Width: Width, Height: height},
		Method: xui.WidthUnicode,
	}))
}

func TestSidebarHiddenByDefault(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	assert.False(t, s.Visible(), "widget waits for the persisted application preference")
	assert.Zero(t, s.ReserveWidth(200), "hidden sidebar reserves no width")
}

func TestSidebarShowsHideShortcut(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	assert.Contains(t, drawText(s, 4), "Ctrl+O hide")
}

func TestSidebarToggleKeyPersistsVisibilityAndReturnsErrors(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	persisted := false
	s.ConfigureVisibility(false, func(visible bool) error {
		persisted = visible
		return nil
	})

	ctx := &components.EventContext{}
	handled, err := s.HandleToggleKey(ctx, ctrlO(false))
	require.NoError(t, err)
	assert.True(t, handled, "Ctrl+O is the sidebar key")
	assert.True(t, s.Visible())
	assert.True(t, persisted)
	assert.True(t, ctx.Consume && ctx.Redraw, "toggle consumes the key and redraws")

	s.ConfigureVisibility(true, func(bool) error { return errors.New("disk full") })
	handled, err = s.HandleToggleKey(ctx, ctrlO(true))
	assert.True(t, handled, "Ctrl+Shift+O toggles too")
	assert.False(t, s.Visible(), "persistence failure must not undo the responsive UI action")
	assert.EqualError(t, err, "disk full")

	ctx = &components.EventContext{}
	handled, err = s.HandleToggleKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'})
	require.NoError(t, err)
	assert.False(t, handled)
	assert.False(t, ctx.Consume, "other keys pass through")
	handled, err = s.HandleToggleKey(
		ctx,
		xui.KeyEvent{Press: false, Code: xui.KeyRune, Rune: 'o', Mods: xui.ModCtrl},
	)
	require.NoError(t, err)
	assert.False(t, handled, "key release is ignored")
}

func TestVisibilityToggleDoesNotDiscardPlan(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.SetPlan(
		session.Plan{Revision: 7, Items: []session.PlanItem{{Content: "keep working", Status: session.PlanInProgress}}},
	)

	assert.False(t, s.Visible())
	assert.Equal(t, uint64(7), s.plan.Revision)
	s.Toggle()
	assert.Contains(t, drawText(s, 20), "keep working")
	s.Toggle()
	assert.False(t, s.Visible())
	assert.Equal(t, uint64(7), s.plan.Revision, "visibility is presentation state, not model-plan state")
}

func TestReserveWidthKeepsChatReadable(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()

	assert.Equal(t, Width, s.ReserveWidth(200), "wide terminal shows the panel")
	assert.Zero(t, s.ReserveWidth(80+Width-1), "chat keeps at least 80 columns")
	assert.Equal(t, Width, s.ReserveWidth(80+Width), "panel fits exactly at the threshold")
	assert.Zero(t, s.ReserveWidth(0))

	s.Toggle()
	assert.Zero(t, s.ReserveWidth(200), "hidden panel never reserves width")
}

func TestDrawContextBar(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.UpdateUsage(session.TokenUsage{PromptTokens: 500, CompletionTokens: 100, TotalTokens: 600})

	txt := drawText(s, 20)
	assert.Contains(t, txt, "context")
	assert.Contains(t, txt, strings.Repeat("█", 10)+strings.Repeat("░", 10)+" 50%", "half-filled bar")
	assert.Contains(t, txt, "500/1.0k", "used over window")
}

func TestDrawContextUnknownUsage(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	txt := drawText(s, 20)
	assert.Contains(t, txt, "awaiting usage")

	noWindow := NewSidebar(components.DefaultTheme(), 0)
	noWindow.Toggle()
	assert.Contains(t, drawText(noWindow, 20), "awaiting usage", "unknown window shows no bar")
}

func TestDrawListsMCPServers(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetServers([]string{"happ", "ozon-mcp"})

	txt := drawText(s, 20)
	assert.Contains(t, txt, "MCP")
	assert.Contains(t, txt, "happ")
	assert.Contains(t, txt, "ozon-mcp")

	empty := NewSidebar(components.DefaultTheme(), 128000)
	empty.Toggle()
	assert.Contains(t, drawText(empty, 20), "none", "no servers configured")
}

func TestDrawListsLSPServers(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetRuntime(Runtime{LSP: []lsp.Language{
		{Language: "go", Server: "gopls", Configured: true, Installed: true, Running: true},
	}})

	txt := drawText(s, 20)
	assert.Contains(t, txt, "LSP")
	assert.Contains(t, txt, "gopls")

	empty := NewSidebar(components.DefaultTheme(), 128000)
	empty.Toggle()
	assert.Contains(t, drawText(empty, 20), "none", "no lsp servers configured")
}

func TestUsageUpdateReplacesTokenRow(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 100000)
	s.Toggle()
	for i := 1; i <= 7; i++ {
		s.UpdateUsage(session.TokenUsage{PromptTokens: 100 * i, TotalTokens: 100 * i})
	}

	txt := drawText(s, 40)
	assert.Contains(t, txt, "tokens")
	assert.Contains(t, txt, "in 700", "latest usage is shown")
	assert.Contains(t, txt, "total 700", "total is shown alongside in")
}

func TestClearUsageResetsCurrentUsage(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 100000)
	s.Toggle()
	s.UpdateUsage(session.TokenUsage{PromptTokens: 1200, CompletionTokens: 800, TotalTokens: 2000})
	s.ClearUsage()

	txt := drawText(s, 20)
	assert.NotContains(t, txt, "total ")
	assert.Contains(t, txt, "awaiting usage")
}

func TestDrawClipsWhenPanelShort(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 100000)
	s.Toggle()
	s.UpdateUsage(session.TokenUsage{PromptTokens: 500, TotalTokens: 600})
	s.SetServers([]string{"happ"})

	txt := drawText(s, 4)
	assert.Contains(t, txt, "context", "context section survives clipping")
	assert.NotContains(t, txt, "happ", "mcp section clips first")
	assert.NotPanics(t, func() { drawText(s, 2) })
}

func TestSetThemeRedraws(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.SetTheme(components.DarkTheme())
	s.Toggle()
	s.UpdateUsage(session.TokenUsage{PromptTokens: 500, TotalTokens: 600})
	assert.Contains(t, drawText(s, 20), "50%")
}

func TestDrawPlanShowsBlockedNotesAndCompletionEvidence(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 4, Items: []session.PlanItem{
		{
			Content: "waiting on user",
			Status:  session.PlanBlocked,
			Note:    "need approval",
		},
		{
			Content:  "verify fix",
			Status:   session.PlanCompleted,
			Evidence: "targeted test passed",
		},
	}})

	txt := drawText(s, 24)
	assert.Contains(t, txt, "! waiting on user")
	assert.Contains(t, txt, "need approval")
	assert.Contains(t, txt, "✓ targeted test passed")
}

func TestPlanViewportScrollsWithoutMovingRuntime(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetRuntime(Runtime{Model: "claude", Mode: "build", Activity: "generating"})
	items := make([]session.PlanItem, 14)
	for i := range items {
		items[i] = session.PlanItem{Content: "step " + strconv.Itoa(i+1), Status: session.PlanPending}
	}
	items[0].Status = session.PlanInProgress
	s.SetPlan(session.Plan{Revision: 1, Items: items})

	before := drawText(s, 20)
	require.Positive(t, s.planHeight)
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: 2, Y: s.planTop})
	after := drawText(s, 20)

	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.Contains(t, after, "context")
	assert.Contains(t, after, "model  claude")
	assert.Contains(t, after, "plan 0/14")
	assert.NotEqual(t, before, after)
	assert.Positive(t, s.planScroll)
}

func TestNewPlanKeepsActiveStepVisibleAndSupportsKeyboardScroll(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	items := make([]session.PlanItem, 14)
	for i := range items {
		items[i] = session.PlanItem{Content: "step " + strconv.Itoa(i+1), Status: session.PlanPending}
	}
	items[len(items)-1].Status = session.PlanInProgress
	s.SetPlan(session.Plan{Revision: 1, Items: items})

	txt := drawText(s, 19)
	assert.Contains(t, txt, "step 14", "new revisions should reveal the active step when the viewport can show it")
	require.Positive(t, s.planScroll)

	before := s.planScroll
	ctx := &components.EventContext{}
	assert.True(t, s.HandleScrollKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyUp, Mods: xui.ModCtrl}))
	assert.Equal(t, before-1, s.planScroll)
	assert.True(t, ctx.Consume && ctx.Redraw)

	s.Toggle()
	assert.False(
		t,
		s.HandleScrollKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyDown, Mods: xui.ModCtrl}),
		"hidden sidebar must not capture composer keys",
	)
}

func TestResizeDragClampsAndCommitsWidth(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	committed := 0
	s.ConfigureWidth(Width, func(width int) error {
		committed = width
		return nil
	})

	press := &components.EventContext{}
	s.Handle(press, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 0})
	require.True(t, press.Consume)

	drag := &components.EventContext{}
	handled, err := s.HandleGlobalMouse(drag, xui.MouseEvent{Action: xui.MouseDrag, Button: xui.MouseLeft, X: 119}, 160)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, 41, s.CurrentWidth())

	release := &components.EventContext{}
	handled, err = s.HandleGlobalMouse(
		release,
		xui.MouseEvent{Action: xui.MouseRelease, Button: xui.MouseLeft, X: 119},
		160,
	)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, 41, committed)
}

func TestSidebarPointerShapeResizeHandle(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	assert.Equal(t, components.ShapeResizeEW, s.PointerShape(0, 3), "border column drags to resize")
	assert.Empty(t, s.PointerShape(2, 3), "panel body keeps the default pointer")
}

func TestPanelTextKeepsGutterFromFrame(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetRuntime(Runtime{Model: "claude", Mode: "build", Activity: "generating"})
	s.UpdateUsage(session.TokenUsage{PromptTokens: 500, TotalTokens: 600})
	s.SetServers([]string{"happ"})
	items := make([]session.PlanItem, 30)
	for i := range items {
		items[i] = session.PlanItem{Content: "step " + strconv.Itoa(i+1), Status: session.PlanPending}
	}
	items[0].Content = "investigate the failing retry path carefully before touching the harness"
	s.SetPlan(session.Plan{Revision: 1, Items: items})

	surf := s.Draw(components.DrawContext{
		Max:    components.Size{Width: Width, Height: 24},
		Method: xui.WidthUnicode,
	})
	require.Contains(t, components.SurfaceText(surf), "model  claude", "content must still render")

	w := surf.Size.Width
	cell := func(x, y int) string { return surf.Buffer[y*w+x].Char }

	assert.Equal(t, " ", cell(1, 1), "first text row must sit one row below the labeled border")
	for y := 1; y < 23; y++ {
		assert.Equal(t, "│", cell(0, y), "row %d: left frame glyph", y)
		assert.Equal(t, "│", cell(w-1, y), "row %d: right frame glyph", y)
		assert.Equal(t, " ", cell(1, y), "row %d: text hugs the left frame", y)
		assert.Contains(t, []string{" ", "│"}, cell(w-2, y), "row %d: text hugs the right frame", y)
	}
}

func TestSidebarApprovalCheckboxTogglesAndCommits(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Approved: false, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit},
	}})
	committed := false
	s.ConfigureApprove(func(approved bool) error {
		committed = approved
		return nil
	})

	drawText(s, 24)
	require.Positive(t, s.approveRowY)
	require.Contains(t, drawText(s, 24), "[ ] approved")

	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: 3, Y: s.approveRowY})
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.True(t, committed)
	assert.True(t, s.approved)
	assert.Contains(t, drawText(s, 24), "[x] approved")
}

func TestSidebarApprovalCtrlAToggles(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Approved: false, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit},
	}})
	committed := false
	s.ConfigureApprove(func(approved bool) error {
		committed = approved
		return nil
	})

	ctx := &components.EventContext{}
	handled, err := s.HandleApproveKey(ctx, xui.KeyEvent{Press: true, Mods: xui.ModCtrl, Code: xui.KeyRune, Rune: 'a'})
	require.NoError(t, err)
	require.True(t, handled)
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.True(t, committed)
	assert.True(t, s.approved)
	assert.Contains(t, drawText(s, 24), "[x] approved")
}

func TestSidebarApprovalCheckboxShowsApprovedState(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Approved: true})
	assert.Contains(t, drawText(s, 24), "[x] approved")
}
