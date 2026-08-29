package sidebar

import (
	"errors"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

func actionPlan() session.Plan {
	return session.Plan{
		Revision: 7, Approved: true, Goal: "ship the fix",
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
			Runs: []session.PlanActionRun{{Status: session.PlanActionRunOK}},
		}},
		Items: []session.PlanItem{
			{
				ID:      "s1",
				Content: "fix the parser",
				Status:  session.PlanPending,
				Type:    session.StepEdit,
				Model:   "plan-b",
				Actions: []session.PlanAction{
					{
						Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
						Runs: []session.PlanActionRun{{Status: session.PlanActionRunFailed, Error: "boom"}},
					},
					{
						Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
						Skills: []string{"tdd", "code-review"},
					},
				},
			},
			{ID: "s2", Content: "read the docs", Status: session.PlanPending, Type: session.StepExplore},
		},
	}
}

func visiblePlanSidebar(t *testing.T) *Sidebar {
	t.Helper()
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetPlan(actionPlan())
	return s
}

// pressStepKey presses one key against the sidebar plan handler and requires
// that the sidebar consumed it.
func pressStepKey(t *testing.T, s *Sidebar, ev xui.KeyEvent) {
	t.Helper()
	ctx := &components.EventContext{}
	handled, err := s.HandlePlanKey(ctx, ev)
	require.True(t, handled, "the sidebar must consume %v", ev.Code)
	require.NoError(t, err)
}

func clickStepLine(t *testing.T, s *Sidebar, idx int) {
	t.Helper()
	_ = drawText(s, 40) // records plan viewport geometry and step spans
	require.NotEmpty(t, s.stepSpans)
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 5, Y: s.planTop + s.stepSpans[idx].start,
	})
}

func drawWide(s *Sidebar, width, height int) string {
	return components.SurfaceText(s.Draw(components.DrawContext{
		Max: components.Size{Width: width, Height: height}, Method: xui.WidthUnicode,
	}))
}

func TestSidebarRendersActionChipsAndModelBadge(t *testing.T) {
	s := visiblePlanSidebar(t)
	// The default 30-column panel wraps the longest chip; a wide panel must
	// show it whole, skills and all.
	s.ConfigureWidth(56, nil)
	text := drawWide(s, 56, 40)

	assert.Contains(t, text, "⚙ compact@step_start", "step chip names action and event")
	assert.Contains(
		t, text, "⚙ inject_skill: tdd, code-review@step_start",
		"an inject_skill chip lists its skills",
	)
	assert.Contains(t, text, "compact@plan_start", "plan-level chip sits under the header")
	assert.Contains(t, text, "◇ plan-b", "the override badge rides the step line")
}

func TestSidebarChipStyleTracksLastRun(t *testing.T) {
	s := visiblePlanSidebar(t)
	theme := components.DefaultTheme()
	lines, _ := s.planContent(contentWidth(s.CurrentWidth()), xui.WidthUnicode)

	var stepChip, planChip *xui.Style
	for i := range lines {
		switch lines[i].text {
		case "  ⚙ compact@step_start":
			stepChip = &lines[i].style
		case "  ⚙ compact@plan_start":
			planChip = &lines[i].style
		}
	}
	require.NotNil(t, stepChip, "step chip line must render")
	require.NotNil(t, planChip, "plan chip line must render")
	assert.Equal(t, theme.Destructive, *stepChip, "a failed last run paints the chip destructive")
	assert.Equal(t, theme.Success, *planChip, "an ok last run paints the chip success")
}

func TestSidebarStepCursorSelectsByClickAndArrows(t *testing.T) {
	s := visiblePlanSidebar(t)
	clickStepLine(t, s, 0)
	assert.Equal(t, 0, s.stepCursor, "a click on the step line selects it")
	assert.True(t, s.planFocus, "a click inside the plan pane focuses it")

	ctx := &components.EventContext{}
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	assert.Equal(t, 1, s.stepCursor, "Down moves the cursor to the next step")
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyUp})
	assert.Equal(t, 0, s.stepCursor)

	assert.Contains(t, drawText(s, 40), "▸", "the focused draw marks the selected step")

	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.False(t, s.planFocus, "Escape drops plan focus")
	handled, err := s.HandlePlanKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	require.NoError(t, err)
	assert.False(t, handled, "arrows pass through once the pane is unfocused")
}

func TestSidebarFocusPlanSelectsFirstStep(t *testing.T) {
	s := visiblePlanSidebar(t)
	require.False(t, s.planFocus)

	require.True(t, s.FocusPlan(), "focus lands when the plan has steps")
	assert.True(t, s.planFocus)
	assert.Equal(t, 0, s.stepCursor, "the first step is selected")

	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	assert.Equal(t, 1, s.stepCursor, "arrows move the selection right after FocusPlan")
}

func TestSidebarPlanPaneHintRow(t *testing.T) {
	s := visiblePlanSidebar(t)
	text := drawText(s, 40)
	assert.Contains(t, text, "alt+P", "the pane names its keyboard entry point")
	assert.Contains(t, text, "m model", "the pane names the picker key")
}

func TestSidebarStepBadgeShowsEffectiveModel(t *testing.T) {
	s := visiblePlanSidebar(t)
	s.SetRuntime(Runtime{Model: "session-default"})
	s.SetPlan(session.Plan{
		Revision:     8,
		ModelsByType: map[session.StepType]string{session.StepExplore: "plan-a"},
		Items: []session.PlanItem{
			{ID: "pinned", Content: "pin", Status: session.PlanPending, Type: session.StepEdit, Model: "plan-b"},
			{ID: "typed", Content: "type", Status: session.PlanPending, Type: session.StepExplore},
			{ID: "plain", Content: "bare", Status: session.PlanPending, Type: session.StepEdit},
		},
	})
	text := drawText(s, 48)
	assert.Contains(t, text, "◇ plan-b", "the step's own pin rides its line")
	assert.Contains(t, text, "◇ plan-a", "a step without a pin shows the type's model")
	assert.Contains(t, text, "◇ session-default", "a step on the session default still shows its effective model")
	assert.Equal(t, 3, strings.Count(text, "◇"), "every step carries a model badge, default included")
}

func TestSidebarPlanFocusDropsOnTyping(t *testing.T) {
	s := visiblePlanSidebar(t)
	require.True(t, s.FocusPlan())

	handled, err := s.HandlePlanKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'})
	require.NoError(t, err)
	assert.False(t, handled, "a plain rune passes through to the composer")
	assert.False(t, s.planFocus, "typing hands the keys back to the composer")
}

func TestSidebarModelPickerListsAndApplies(t *testing.T) {
	s := visiblePlanSidebar(t)
	s.ConfigureModels([]string{"plan-a", "plan-b"})
	var gotStep, gotModel string
	s.ConfigureStepModel(func(stepID, model string) error {
		gotStep, gotModel = stepID, model
		return nil
	})

	clickStepLine(t, s, 0)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})

	text := drawText(s, 40)
	assert.Contains(t, text, "step type default", "entry zero clears the override")
	assert.Contains(t, text, "plan-a")
	assert.Contains(t, text, "plan-b")
	require.Equal(t, 2, s.pickerCursor, "the pinned model is preselected")

	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	assert.Equal(t, "s1", gotStep)
	assert.Equal(t, "plan-b", gotModel)
	assert.NotContains(t, drawText(s, 40), "step type default", "Enter closes the picker")
}

func TestSidebarModelPickerWrapsAndClears(t *testing.T) {
	s := visiblePlanSidebar(t)
	s.ConfigureModels([]string{"plan-a", "plan-b"})
	var gotModel string
	calls := 0
	s.ConfigureStepModel(func(_, model string) error {
		calls++
		gotModel = model
		return nil
	})

	clickStepLine(t, s, 1)
	// Enter opens the picker for the selected step too.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.Equal(t, 0, s.pickerCursor, "a step without a pin starts on the clear entry")

	// Up from entry zero wraps to the last model.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyUp})
	require.Equal(t, 2, s.pickerCursor)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	assert.Equal(t, 1, calls)
	assert.Equal(t, "plan-b", gotModel)
}

func TestSidebarModelPickerCancelAndClickAway(t *testing.T) {
	s := visiblePlanSidebar(t)
	calls := 0
	s.ConfigureStepModel(func(string, string) error {
		calls++
		return nil
	})

	clickStepLine(t, s, 0)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})

	// Escape cancels without a commit.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.False(t, s.pickerOpen)
	assert.Zero(t, calls)

	// Reopen, then a click away closes it too.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 5, Y: s.planTop,
	})
	assert.False(t, s.pickerOpen)
	assert.Zero(t, calls, "closing the picker commits nothing")
}

func TestSidebarModelPickerCommitErrorKeepsPicker(t *testing.T) {
	s := visiblePlanSidebar(t)
	s.ConfigureStepModel(func(string, string) error { return errors.New("stale revision") })

	clickStepLine(t, s, 0)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})

	ctx := &components.EventContext{}
	handled, err := s.HandlePlanKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.True(t, handled)
	require.EqualError(t, err, "stale revision", "commit failures surface like every sidebar commit")
	assert.True(t, s.pickerOpen, "a failed commit keeps the picker open for another try")
}
