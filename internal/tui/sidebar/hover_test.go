package sidebar

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

// hoverDraw renders one frame with the pointer resting on (x, y), the way
// the app publishes hover state into DrawContext.
func hoverDraw(s *Sidebar, x, y int) components.Surface {
	return s.Draw(components.DrawContext{
		Max:    components.Size{Width: Width, Height: 40},
		Method: xui.WidthUnicode,
		Hover:  &components.HoverState{Widget: s, X: x, Y: y},
	})
}

// bgAt reads one cell's background.
func bgAt(s components.Surface, x, y int) xui.Color {
	return s.Buffer[y*s.Size.Width+x].Style.Bg
}

// tinted asserts the whole rect [x0, x1) on row y carries the element tint.
func tinted(t *testing.T, s components.Surface, y, x0, x1 int) {
	t.Helper()
	want := components.DefaultTheme().BackgroundElement.Bg
	for x := x0; x < x1; x++ {
		assert.Equal(t, want, bgAt(s, x, y), "col %d on row %d", x, y)
	}
}

func notTinted(t *testing.T, s components.Surface, y, x0, x1 int) {
	t.Helper()
	want := components.DefaultTheme().BackgroundElement.Bg
	for x := x0; x < x1; x++ {
		assert.NotEqual(t, want, bgAt(s, x, y), "col %d on row %d", x, y)
	}
}

// Every clickable control offers the hand and lights exactly its own cells:
// the tab pair, the approval row's three segments, the settings toggles, and
// a skill row. Non-controls keep the default shape and stay quiet.
func TestSidebarControlsHoverTint(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetPlan(session.Plan{Revision: 1, Items: []session.PlanItem{{
		ID:      "step-1",
		Content: "do the thing",
		Actions: []session.PlanAction{{Type: session.PlanActionInjectSkill, Skills: []string{"tdd"}}},
	}}})
	s.Draw(components.DrawContext{Max: components.Size{Width: Width, Height: 40}, Method: xui.WidthUnicode})

	// Tabs: the settings tab lights exactly its own columns, and the gap
	// between the tabs is no control at all.
	require.GreaterOrEqual(t, s.tabRowY, 0)
	assert.Equal(t, hoverSettingsTab, s.HoverRegion(s.settingsTabMinX, s.tabRowY))
	assert.Equal(t, components.ShapePointer, s.PointerShape(s.settingsTabMinX, s.tabRowY))
	tinted(t, hoverDraw(s, s.settingsTabMinX, s.tabRowY), s.tabRowY, s.settingsTabMinX, s.settingsTabMaxX)
	notTinted(t, hoverDraw(s, s.settingsTabMinX, s.tabRowY), s.tabRowY, s.statusTabMinX, s.statusTabMaxX)
	gap := s.statusTabMaxX
	assert.Zero(t, s.HoverRegion(gap, s.tabRowY))
	assert.Empty(t, s.PointerShape(gap, s.tabRowY))

	// The approval row is three controls on one row: each segment lights
	// alone, exactly where a click would land.
	require.GreaterOrEqual(t, s.approveRowY, 0)
	require.Greater(t, s.clearToggleX, s.autoToggleX)
	assert.Equal(t, hoverApprove, s.HoverRegion(1, s.approveRowY))
	tinted(t, hoverDraw(s, 1, s.approveRowY), s.approveRowY, 1, s.autoToggleX)
	assert.Equal(t, hoverAuto, s.HoverRegion(s.autoToggleX, s.approveRowY))
	autoFrame := hoverDraw(s, s.autoToggleX, s.approveRowY)
	tinted(t, autoFrame, s.approveRowY, s.autoToggleX, s.clearToggleX)
	notTinted(t, autoFrame, s.approveRowY, 1, s.autoToggleX)
	assert.Equal(t, hoverClear, s.HoverRegion(s.clearToggleX, s.approveRowY))
	tinted(t, hoverDraw(s, s.clearToggleX, s.approveRowY), s.approveRowY, s.clearToggleX, Width-1-panelPad)

	// Settings tab: toggle rows tint across the content row, chips exactly
	// their own column.
	s.setTab(tabSettings)
	s.Draw(components.DrawContext{Max: components.Size{Width: Width, Height: 40}, Method: xui.WidthUnicode})
	require.GreaterOrEqual(t, s.stopRowY, 0)
	assert.Equal(t, hoverStop, s.HoverRegion(2, s.stopRowY))
	tinted(t, hoverDraw(s, 2, s.stopRowY), s.stopRowY, 1+panelPad, Width-1-panelPad)
	assert.Equal(t, hoverPlanToggle, s.HoverRegion(2, s.planRowY))
	assert.Equal(t, hoverEdits, s.HoverRegion(2, s.editsRowY))
	require.GreaterOrEqual(t, s.mainMinusX, 0)
	assert.Equal(t, hoverMainMinus, s.HoverRegion(s.mainMinusX, s.mainCtxRowY))
	tinted(t, hoverDraw(s, s.mainMinusX, s.mainCtxRowY), s.mainCtxRowY, s.mainMinusX, s.mainMinusX+1)
	notTinted(t, hoverDraw(s, s.mainMinusX, s.mainCtxRowY), s.mainCtxRowY, s.mainMinusX+1, s.mainPlusX)
	assert.Equal(t, hoverMainPlus, s.HoverRegion(s.mainPlusX, s.mainCtxRowY))
	assert.Equal(t, hoverAgentsMinus, s.HoverRegion(s.agentsMinusX, s.agentsCtxRowY))
	assert.Equal(t, hoverAgentsPlus, s.HoverRegion(s.agentsPlusX, s.agentsCtxRowY))

	// The plan pane: a skill row is a control, the step's own line is not.
	s.setTab(tabStatus)
	s.Draw(components.DrawContext{Max: components.Size{Width: Width, Height: 40}, Method: xui.WidthUnicode})
	require.NotEmpty(t, s.skillHits)
	skill := s.skillHits[0]
	skillRow := s.planTop + skill.line - s.planScroll
	assert.Equal(t, hoverSkillRowBase+skill.line, s.HoverRegion(2, skillRow))
	tinted(t, hoverDraw(s, 2, skillRow), skillRow, 1+panelPad, Width-1-panelPad)
	stepRow := s.planTop + s.stepSpans[0].start - s.planScroll
	assert.Zero(t, s.HoverRegion(2, stepRow))
	assert.Empty(t, s.PointerShape(2, stepRow))
}
