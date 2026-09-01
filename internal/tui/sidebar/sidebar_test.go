package sidebar

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/session"
)

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
	handled, err := s.ToggleVisibility(ctx)
	require.NoError(t, err)
	assert.True(t, handled, "the toggle applies whenever a sidebar exists")
	assert.True(t, s.Visible())
	assert.True(t, persisted)
	assert.True(t, ctx.Consume && ctx.Redraw, "toggle consumes the key and redraws")

	s.ConfigureVisibility(true, func(bool) error { return errors.New("disk full") })
	handled, err = s.ToggleVisibility(ctx)
	assert.True(t, handled)
	assert.False(t, s.Visible(), "persistence failure must not undo the responsive UI action")
	assert.EqualError(t, err, "disk full")
}

func TestVisibilityToggleDoesNotDiscardPlan(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.SetPlan(
		session.Plan{Revision: 7, Items: []session.PlanItem{{Content: "keep working", Status: session.PlanInProgress}}},
	)

	assert.False(t, s.Visible())
	assert.Equal(t, uint64(7), s.plan.Revision)
	s.Toggle()
	assert.Contains(t, drawText(s, 28), "keep working")
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

func TestDrawLeadsWithSessionModel(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetRuntime(Runtime{Model: "claude"})

	txt := drawText(s, 20)
	assert.Contains(t, txt, "claude", "the session model leads the status tab")
	assert.Less(
		t,
		strings.Index(txt, "claude"),
		strings.Index(txt, "context"),
		"model renders above the context section",
	)
	assert.NotContains(t, txt, "skills", "the status tab lists no skills; the plan carries them")

	unset := NewSidebar(components.DefaultTheme(), 128000)
	unset.Toggle()
	assert.Contains(t, drawText(unset, 20), "(unset)", "an unknown model is said, not hidden")
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

	txt := drawText(s, 8)
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

	txt := drawText(s, 32)
	assert.Contains(t, txt, "! waiting on user")
	assert.Contains(t, txt, "need approval")
	assert.Contains(t, txt, "✓ targeted test passed")
}

// TestSidebarBlockedStepShowsResumeConditionInDetails pins the blocked pair:
// the brief view carries the blocker, the expanded view adds the resume
// condition the block transition owns — and both views survive the
// narrowest width.
func TestSidebarBlockedStepShowsResumeConditionInDetails(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 3, Items: []session.PlanItem{
		{
			Content:    "waiting on user",
			Status:     session.PlanBlocked,
			Blocker:    "need approval",
			ResumeWhen: "user answers",
		},
	}})

	brief := drawText(s, 24)
	assert.Contains(t, brief, "! waiting on user")
	assert.Contains(t, brief, "need approval")
	assert.NotContains(t, brief, "user answers", "the resume condition is detail, not brief")
	assert.NotPanics(t, func() { drawText(s, 20) })

	require.True(t, s.TogglePlanDetails(&components.EventContext{}))

	detailed := drawText(s, 30)
	assert.Contains(t, detailed, "need approval")
	assert.Contains(t, detailed, "resume: user answers")
	assert.NotPanics(t, func() { drawText(s, 20) })
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

	before := drawText(s, 28)
	require.Positive(t, s.planHeight)
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: 2, Y: s.planTop})
	after := drawText(s, 28)

	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.Contains(t, after, "context")
	assert.Contains(t, after, "claude", "the session model always rides the sidebar, default included")
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

	txt := drawText(s, 28)
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
	require.Contains(t, components.SurfaceText(surf), "context", "content must still render")

	w := surf.Size.Width
	cell := func(x, y int) string { return surf.Buffer[y*w+x].Char }

	assert.Equal(t, " ", cell(1, 1), "first text row must sit one row below the labeled border")
	dividerRow := -1
	for y := 1; y < 23; y++ {
		if cell(0, y) == "├" {
			dividerRow = y
			assert.Equal(t, "┤", cell(w-1, y), "divider row closes on the right frame")
			continue
		}
		assert.Equal(t, "│", cell(0, y), "row %d: left frame glyph", y)
		assert.Equal(t, "│", cell(w-1, y), "row %d: right frame glyph", y)
		assert.Equal(t, " ", cell(1, y), "row %d: text hugs the left frame", y)
		assert.Contains(t, []string{" ", "│"}, cell(w-2, y), "row %d: text hugs the right frame", y)
	}
	require.Positive(t, dividerRow, "the plan pane must render its divider")
	for x := 1; x < w-1; x++ {
		assert.Equal(t, " ", cell(x, dividerRow-1), "one blank row must separate the tab window from the plan pane")
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
	handled, err := s.TogglePlanApproved(ctx)
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

func TestSidebarAutoToggle(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Approved: false, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit},
	}})
	drawText(s, 24)
	require.Positive(t, s.autoRowY)
	require.Positive(t, s.autoToggleX)
	require.Contains(t, drawText(s, 24), "[ ] auto")
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: s.autoToggleX, Y: s.autoRowY})
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.True(t, s.AutoApprove())
	assert.Contains(t, drawText(s, 24), "[x] auto")
}

func TestSidebarClearPlanInvokesCallbackAndClears(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.width = 44 // default 30 is too narrow to fit the full "clear" label; widen to prove the row renders
	s.SetPlan(session.Plan{Revision: 3, Approved: true, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit},
	}})
	drawText(s, 24)
	require.Positive(t, s.autoRowY)
	require.Positive(t, s.clearToggleX)
	require.Contains(t, drawText(s, 24), "clear")

	cleared := false
	s.ConfigureClearPlan(func() error {
		cleared = true
		s.SetPlan(session.Plan{Revision: 0, Items: nil})
		return nil
	})

	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: s.clearToggleX, Y: s.approveRowY})
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.True(t, cleared, "clear invokes the bound callback")
	assert.Contains(t, drawText(s, 24), "No plan yet", "cleared plan shows the empty viewport")
}

func TestSidebarClearWithoutCallbackStillConsumes(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanPending},
	}})
	drawText(s, 24)
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, X: s.clearToggleX, Y: s.approveRowY})
	assert.True(t, ctx.Consume && ctx.Redraw)
}

func TestSidebarStopTogglePersistsAndApplies(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.setTab(tabSettings)

	persisted := false
	s.ConfigureStopOnLimit(true, func(enabled bool) error {
		persisted = enabled
		return nil
	})

	ctx := &components.EventContext{}
	require.NoError(t, s.toggleStop(ctx))
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.False(t, persisted, "onStopCommit receives the flipped value")
	assert.False(t, s.StopOnLimit())
	assert.Contains(t, drawText(s, 24), "[ ] stop@128")
}

func TestSidebarPlanFeatureTogglePersistsAndHidesPlanPane(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Items: []session.PlanItem{
		{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit},
	}})
	persisted := true
	s.ConfigurePlanFeature(true, func(enabled bool) error {
		persisted = enabled
		return nil
	})

	require.True(t, s.PlanEnabled())
	require.Contains(t, drawText(s, 24), "approved", "the plan pane renders while the feature is on")
	s.setTab(tabSettings)
	require.Contains(t, drawText(s, 24), "[x] plan")

	ctx := &components.EventContext{}
	require.NoError(t, s.togglePlanFeature(ctx))
	assert.True(t, ctx.Consume && ctx.Redraw)
	assert.False(t, persisted, "onPlanCommit receives the flipped value")
	assert.False(t, s.PlanEnabled())
	assert.Contains(t, drawText(s, 24), "[ ] plan")

	s.setTab(tabStatus)
	disabled := drawText(s, 24)
	assert.NotContains(t, disabled, "approved", "the approval row hides with the plan pane")
	assert.NotContains(t, disabled, "step", "the plan viewport hides with the plan pane")
	assert.Equal(t, 0, s.planHeight, "no viewport is exposed while the feature is off")
	assert.Equal(t, -1, s.approveRowY, "no approval hit-test row while the feature is off")

	// Ctrl+A belongs to the plan feature: with it off, the key falls through.
	keyCtx := &components.EventContext{}
	handled, err := s.TogglePlanApproved(keyCtx)
	require.NoError(t, err)
	assert.False(t, handled, "Ctrl+A is inert while the plan feature is off")
	assert.False(t, keyCtx.Consume)
}

func TestSidebarTabSwitchSwapsTopWindowOnly(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetRuntime(Runtime{Model: "claude"})
	s.UpdateUsage(session.TokenUsage{PromptTokens: 500, TotalTokens: 600})

	status := drawText(s, 24)
	require.Contains(t, status, "context")
	require.NotContains(t, status, "stop@128")

	planTop, planHeight := s.planTop, s.planHeight
	s.setTab(tabSettings)
	settings := drawText(s, 24)
	assert.Contains(t, settings, "stop@128")
	assert.NotContains(t, settings, "context")
	assert.Equal(t, planTop, s.planTop, "switching tabs must not move the plan pane")
	assert.Equal(t, planHeight, s.planHeight, "switching tabs must not resize the plan pane")
	assert.Contains(t, settings, "approved", "plan approval stays visible on both tabs")
	assert.Contains(t, settings, "plan", "the plan divider stays with the plan pane")
}

func TestSidebarSetPlanMirrorsDurableApprovalWithoutCommitting(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.autoApprove.Store(true)
	callbacks := 0
	s.ConfigureApprove(func(bool) error {
		callbacks++
		return nil
	})
	items := []session.PlanItem{{Content: "step", Status: session.PlanInProgress, Type: session.StepEdit}}

	s.SetPlan(session.Plan{Revision: 1, Approved: false, Items: items})
	assert.False(t, s.approved)
	assert.Contains(t, drawText(s, 24), "[ ] approved")
	assert.Contains(t, drawText(s, 24), "[x] auto")

	s.SetPlan(session.Plan{Revision: 2, Approved: true, Items: items})
	assert.True(t, s.approved)
	assert.Contains(t, drawText(s, 24), "[x] approved")

	s.SetPlan(session.Plan{Revision: 3, Approved: false, Items: []session.PlanItem{{
		Content: "step", Status: session.PlanCompleted, Type: session.StepEdit,
	}}})
	assert.False(t, s.approved)
	assert.Zero(t, callbacks, "rendering a durable snapshot must not mutate it")
}

// TestSidebarPlanDefaultViewShowsGoalProgressActiveAndBlockers pins the
// brief default view of a v2 contract: the goal, the progress divider, the
// active step and blocker reasons render — and no rationale field leaks in
// until the user expands one.
func TestSidebarPlanDefaultViewShowsGoalProgressActiveAndBlockers(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{
		Revision: 2,
		Schema:   session.PlanSchemaV2,
		Goal:     "land the release checklist",
		Items: []session.PlanItem{
			{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
			{ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress, Why: "the tag is cut"},
			{ID: "wait", Content: "wait for sign-off", Status: session.PlanBlocked, Blocker: "registry is down"},
			{ID: "note", Content: "note the outcome", Status: session.PlanPending},
		},
	})

	txt := drawText(s, 40)
	assert.Contains(t, txt, "land the release checklist")
	assert.Contains(t, txt, "1/4", "the divider counts finished work")
	assert.Contains(t, txt, "ship the checklist")
	assert.Contains(t, txt, "registry is down", "a blocked step carries its reason")
	assert.NotContains(t, txt, "the tag is cut", "rationale stays out of the brief view")
}

// TestSidebarPlanDetailsKeyExpandsRationale pins the expanded view: Ctrl+D
// flips the pane to details — plan approach and working context, per-step why,
// done_when, outcome and evidence refs — and flips back to the brief view.
// Long detail content scrolls in the same viewport instead of a second one.
func TestSidebarPlanDetailsKeyExpandsRationale(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{
		Revision:       2,
		Schema:         session.PlanSchemaV2,
		Goal:           "land the release checklist",
		Approach:       "audit, then ship",
		WorkingContext: "worktree clean",
		Items: []session.PlanItem{
			{
				ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted,
				Outcome: "3 gaps found", EvidenceRefs: []string{"cmd:a1b2"},
			},
			{
				ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress,
				Why: "tag is cut", DoneWhen: "tag on origin",
			},
		},
	})

	ctx := &components.EventContext{}
	brief := drawText(s, 40)
	assert.NotContains(t, brief, "audit, then ship", "approach is detail, not brief")

	handled := s.TogglePlanDetails(ctx)
	require.True(t, handled, "the details command toggles the view")
	require.True(t, ctx.Consume && ctx.Redraw, "the toggle consumes the key and redraws")

	detailed := drawText(s, 40)
	assert.Contains(t, detailed, "audit, then ship")
	assert.Contains(t, detailed, "worktree clean")
	assert.Contains(t, detailed, "tag is cut")
	assert.Contains(t, detailed, "tag on origin")
	assert.Contains(t, detailed, "3 gaps found")
	assert.Contains(t, detailed, "cmd:a1b2")

	s.TogglePlanDetails(&components.EventContext{})
	assert.NotContains(t, drawText(s, 40), "audit, then ship", "a second press returns to brief")
}

// TestSidebarPlanMarkersDeriveFromCanonicalStatus renders one step per
// lifecycle status and pins a distinct marker for each: the icon IS the
// canonical status, computed at draw time — never a separately stored
// checked state that could survive a status change.
func TestSidebarPlanMarkersDeriveFromCanonicalStatus(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 3, Items: []session.PlanItem{
		{ID: "todo", Content: "pending step", Status: session.PlanPending},
		{ID: "now", Content: "active step", Status: session.PlanInProgress},
		{ID: "done", Content: "done step", Status: session.PlanCompleted},
		{ID: "stuck", Content: "blocked step", Status: session.PlanBlocked},
		{ID: "gone", Content: "cancelled step", Status: session.PlanCancelled},
	}})

	txt := drawText(s, 40)
	assert.Contains(t, txt, "○ pending step")
	assert.Contains(t, txt, "● active step")
	assert.Contains(t, txt, "✓ done step")
	assert.Contains(t, txt, "! blocked step")
	assert.Contains(t, txt, "– cancelled step")

	// The same step id completing in the next snapshot moves its marker:
	// nothing per-step is cached across renders.
	s.SetPlan(session.Plan{Revision: 4, Items: []session.PlanItem{
		{ID: "now", Content: "active step", Status: session.PlanCompleted},
	}})
	txt = drawText(s, 40)
	assert.Contains(t, txt, "✓ active step")
	assert.NotContains(t, txt, "● active step")
}

// TestSidebarReapprovalShowsBoundedMaterialDiff pins the reapproval block:
// when a material revision revokes the user's approval, the pane surfaces
// exactly the bounded diff — which fields moved on which targets — and never
// echoes the replaced prose.
func TestSidebarReapprovalShowsBoundedMaterialDiff(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	s.SetPlan(session.Plan{
		Revision: 1,
		Schema:   session.PlanSchemaV2,
		Approved: true,
		Goal:     "land the release checklist",
		Items: []session.PlanItem{
			{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
			{ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress, Risk: "small radius"},
		},
	})

	// The model revises the contract: a new goal and a sharper risk revoke
	// the approval; everything else stays operational.
	revised := session.Plan{
		Revision: 2,
		Schema:   session.PlanSchemaV2,
		Approved: false,
		Goal:     "ship faster",
		Items: []session.PlanItem{
			{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
			{ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress, Risk: "wide radius"},
		},
	}
	s.SetPlan(revised)

	txt := drawText(s, 48)
	assert.Contains(t, txt, "reapproval: 2 changes")
	assert.Contains(t, txt, "plan.goal")
	assert.Contains(t, txt, "ship.risk")
	assert.NotContains(t, txt, "land the release checklist", "the replaced prose never echoes")
}

// TestSidebarReapprovalAnchorsLastApprovedPlan pins the diff anchor: while a
// material revision sits unapproved, later operational updates must not
// advance the anchor — the diff still compares against what the user last
// approved, so reapproval cannot silently vanish under plan churn.
func TestSidebarReapprovalAnchorsLastApprovedPlan(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	base := session.Plan{
		Revision: 5,
		Schema:   session.PlanSchemaV2,
		Goal:     "land the release checklist",
		Approved: true,
		Items: []session.PlanItem{
			{ID: "audit", Content: "audit the checklist", Status: session.PlanInProgress},
			{ID: "ship", Content: "ship the checklist", Status: session.PlanPending},
		},
	}
	s.SetPlan(base)

	revised := base
	revised.Revision = 6
	revised.Approved = false
	revised.Goal = "ship the release checklist"
	s.SetPlan(revised)
	assert.Contains(t, drawText(s, 48), "reapproval: 1 changes", "first material revision shows its diff")

	progressed := revised
	progressed.Revision = 7
	progressed.Items = []session.PlanItem{
		{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
		{ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress},
	}
	s.SetPlan(progressed)

	txt := drawText(s, 48)
	assert.Contains(t, txt, "reapproval: 1 changes", "an operational update while unapproved keeps the diff anchored")
	assert.Contains(t, txt, "plan.goal")
	assert.Contains(t, txt, "✓ audit the checklist", "the newer status still renders")
}

// TestSidebarPlanTransitionsKeepViewerStable pins the viewer across plan
// lifecycle transitions: a closed plan still says how it ended, a reopened
// plan keeps the expanded details view the user chose, and the result line
// leaves when the closed state does.
func TestSidebarPlanTransitionsKeepViewerStable(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.Toggle()
	closedAt := time.Now()
	closed := session.Plan{
		Revision: 7,
		Schema:   session.PlanSchemaV2,
		Goal:     "land the release checklist",
		Approach: "audit, then ship",
		Result:   session.PlanResultSuccess,
		ClosedAt: &closedAt,
		Items: []session.PlanItem{
			{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
			{ID: "ship", Content: "ship the checklist", Status: session.PlanCompleted},
		},
	}
	s.SetPlan(closed)

	txt := drawText(s, 30)
	assert.Contains(t, txt, "closed: success", "a finished plan says how it ended")
	assert.NotPanics(t, func() { drawText(s, 24) })

	require.True(t, s.TogglePlanDetails(&components.EventContext{}))

	reopened := closed
	reopened.Revision = 8
	reopened.Approved = true
	reopened.Result = ""
	reopened.ClosedAt = nil
	reopened.Items = []session.PlanItem{
		{ID: "audit", Content: "audit the checklist", Status: session.PlanCompleted},
		{ID: "ship", Content: "ship the checklist", Status: session.PlanInProgress},
	}
	s.SetPlan(reopened)

	assert.True(t, s.planDetails, "the expanded view survives a plan transition")
	reopenedTxt := drawText(s, 30)
	assert.Contains(t, reopenedTxt, "audit, then ship", "details still render after reopen")
	assert.NotContains(t, reopenedTxt, "closed: success", "the result line leaves with the closed state")
	assert.NotPanics(t, func() { drawText(s, 24) })
}

func TestSetRuntimeDropsUnchangedSnapshot(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 1000)
	s.SetRuntime(
		Runtime{Model: "m", MCP: []mcp.ServerStatus{{Name: "srv", State: mcp.StateConnected}}},
	)
	stored := s.runtime.MCP

	// The editor pushes the full controller snapshot every frame; an equal
	// push from a fresh backing array must not re-clone or replace.
	s.SetRuntime(
		Runtime{Model: "m", MCP: []mcp.ServerStatus{{Name: "srv", State: mcp.StateConnected}}},
	)
	require.Len(t, s.runtime.MCP, 1)
	assert.Same(t, &stored[0], &s.runtime.MCP[0], "equal snapshot keeps the stored slice")

	// Deep difference inside an LSP entry (Operations) still counts as changed.
	s.SetRuntime(Runtime{Model: "m", LSP: []lsp.Language{{Language: "go"}}})
	s.SetRuntime(Runtime{
		Model: "m",
		LSP:   []lsp.Language{{Language: "go", Operations: []string{"hover"}}},
	})
	assert.Equal(t, []string{"hover"}, s.runtime.LSP[0].Operations)
}
