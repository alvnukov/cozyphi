package sidebar

import (
	"errors"
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
				Actions: []session.PlanAction{{
					Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
					Runs: []session.PlanActionRun{{Status: session.PlanActionRunFailed, Error: "boom"}},
				}},
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
func pressStepKey(t *testing.T, s *Sidebar, ev xui.KeyEvent) error {
	t.Helper()
	ctx := &components.EventContext{}
	handled, err := s.HandlePlanKey(ctx, ev)
	require.True(t, handled, "the sidebar must consume %v", ev.Code)
	require.NoError(t, err)
	return nil
}

func clickStepLine(t *testing.T, s *Sidebar, idx int) {
	t.Helper()
	_ = drawText(s, 40) // records plan viewport geometry and step spans
	require.NotEmpty(t, s.stepSpans)
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 5, Y: s.planTop + s.stepSpans[idx].start,
	})
}

func TestSidebarRendersActionChipsAndModelBadge(t *testing.T) {
	s := visiblePlanSidebar(t)
	text := drawText(s, 40)

	assert.Contains(t, text, "⚙ compact@step_start", "step chip names action and event")
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
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyDown}))
	assert.Equal(t, 1, s.stepCursor, "Down moves the cursor to the next step")
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyUp}))
	assert.Equal(t, 0, s.stepCursor)

	assert.Contains(t, drawText(s, 40), "▸", "the focused draw marks the selected step")

	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEscape}))
	assert.False(t, s.planFocus, "Escape drops plan focus")
	handled, err := s.HandlePlanKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	require.NoError(t, err)
	assert.False(t, handled, "arrows pass through once the pane is unfocused")
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
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'}))

	text := drawText(s, 40)
	assert.Contains(t, text, "step type default", "entry zero clears the override")
	assert.Contains(t, text, "plan-a")
	assert.Contains(t, text, "plan-b")
	require.Equal(t, 2, s.pickerCursor, "the pinned model is preselected")

	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))
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
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter}),
		"Enter opens the picker for the selected step too")
	require.Equal(t, 0, s.pickerCursor, "a step without a pin starts on the clear entry")

	// Up from entry zero wraps to the last model.
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyUp}))
	require.Equal(t, 2, s.pickerCursor)
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))
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
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))

	// Escape cancels without a commit.
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEscape}))
	assert.False(t, s.pickerOpen)
	assert.Zero(t, calls)

	// Reopen, then a click away closes it too.
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))
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
	require.NoError(t, pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'}))

	ctx := &components.EventContext{}
	handled, err := s.HandlePlanKey(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.True(t, handled)
	require.EqualError(t, err, "stale revision", "commit failures surface like every sidebar commit")
	assert.True(t, s.pickerOpen, "a failed commit keeps the picker open for another try")
}
