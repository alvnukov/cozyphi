package sidebar

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
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
	assert.False(t, s.Visible(), "sidebar starts hidden like opencode")
	assert.Zero(t, s.ReserveWidth(200), "hidden sidebar reserves no width")
}

func TestSidebarToggleKey(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)

	ctx := &components.EventContext{}
	assert.True(t, s.HandleToggleKey(ctx, ctrlO(false)), "Ctrl+O is the sidebar key")
	assert.True(t, s.Visible())
	assert.True(t, ctx.Consume && ctx.Redraw, "toggle consumes the key and redraws")

	assert.True(t, s.HandleToggleKey(ctx, ctrlO(true)), "Ctrl+Shift+O toggles too")
	assert.False(t, s.Visible())

	ctx = &components.EventContext{}
	assert.False(t, s.HandleToggleKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'}))
	assert.False(t, ctx.Consume, "other keys pass through")
	assert.False(
		t,
		s.HandleToggleKey(ctx, xui.KeyEvent{Press: false, Code: xui.KeyRune, Rune: 'o', Mods: xui.ModCtrl}),
		"key release is ignored",
	)
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
	assert.Contains(t, txt, "mcp")
	assert.Contains(t, txt, "happ")
	assert.Contains(t, txt, "ozon-mcp")

	empty := NewSidebar(components.DefaultTheme(), 128000)
	empty.Toggle()
	assert.Contains(t, drawText(empty, 20), "none", "no servers configured")
}

func TestUsageHistoryNewestFirstCapped(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 100000)
	s.Toggle()
	for i := 1; i <= 7; i++ {
		s.UpdateUsage(session.TokenUsage{PromptTokens: 100 * i, TotalTokens: 100 * i})
	}

	txt := drawText(s, 40)
	assert.Contains(t, txt, "tokens")
	require.NotEmpty(t, strings.Index(txt, "↑700"))
	assert.Contains(t, txt, "↑300", "last five turns are kept")
	assert.NotContains(t, txt, "↑100", "older turns drop off")
	assert.NotContains(t, txt, "↑200")
	assert.Less(t, strings.Index(txt, "↑700"), strings.Index(txt, "↑300"), "newest turn first")
}

func TestClearUsageResetsHistory(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 100000)
	s.Toggle()
	s.UpdateUsage(session.TokenUsage{PromptTokens: 1200, CompletionTokens: 800, TotalTokens: 2000})
	s.ClearUsage()

	txt := drawText(s, 20)
	assert.NotContains(t, txt, "↑")
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

	txt := drawText(s, 16)
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
