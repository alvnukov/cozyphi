package sidebar

import (
	"errors"
	"fmt"
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
	assert.Contains(t, text, "○ tdd", "a not-yet-run approved skill reads as a hollow green circle")
	assert.Contains(t, text, "○ code-review", "each skill gets its own indented row")
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

	s.SetRuntime(Runtime{Model: "live-engine", SessionModel: "session-default"})
	text = drawText(s, 48)
	assert.Contains(t, text, "◇ session-default", "unpinned steps ride the session default, not the live engine label")
	assert.NotContains(t, text, "◇ live-engine", "the live engine label belongs to the status area, not the badge")
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

func TestSidebarModelPickerPageAndVimKeys(t *testing.T) {
	s := visiblePlanSidebar(t)
	models := make([]string, 40)
	for i := range models {
		models[i] = fmt.Sprintf("model-%02d", i)
	}
	s.ConfigureModels(models)
	var gotModel string
	s.ConfigureStepModel(func(_, model string) error {
		gotModel = model
		return nil
	})

	clickStepLine(t, s, 0)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})
	require.True(t, s.pickerOpen)
	// g normalizes the preselection so the paging math below is stable.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'g'})
	require.Zero(t, s.pickerCursor)

	// Shrink the pane so the overlay's visible window holds a handful of
	// rows; a page is that window minus one overlap row.
	_ = drawText(s, 30)
	entries := len(models) + 1 // the clear entry rides on top
	rows := min(entries, max(s.planHeight-2, 1))
	require.GreaterOrEqual(t, rows, 3, "the pane must show at least three picker rows for a paging test")
	step := rows - 1

	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyPageDown})
	assert.Equal(t, step, s.pickerCursor)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyPageUp})
	assert.Zero(t, s.pickerCursor)

	// Page keys clamp at both ends instead of wrapping.
	for range entries {
		pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyPageDown})
	}
	assert.Equal(t, entries-1, s.pickerCursor)
	for range entries {
		pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyPageUp})
	}
	assert.Zero(t, s.pickerCursor)

	// Vim keys: G/g jump to the ends, j/k step and wrap like the arrows.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'G'})
	assert.Equal(t, entries-1, s.pickerCursor)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'j'})
	assert.Zero(t, s.pickerCursor, "j wraps from the last entry to the first")
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'k'})
	assert.Equal(t, entries-1, s.pickerCursor, "k wraps from the first entry to the last")
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'g'})
	assert.Zero(t, s.pickerCursor)

	// G plus Enter commits the last model, proving the cursor indexes the
	// entries it moved over.
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'G'})
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	assert.False(t, s.pickerOpen)
	assert.Equal(t, models[len(models)-1], gotModel)

	// Any other rune still abandons the picker for the composer.
	clickStepLine(t, s, 0)
	pressStepKey(t, s, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'm'})
	require.True(t, s.pickerOpen)
	handled, err := s.HandlePlanKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'x'})
	require.NoError(t, err)
	assert.False(t, handled, "unmapped runes still fall through to the composer")
	assert.False(t, s.pickerOpen)
}

func numberedPlan(n int, rev uint64) session.Plan {
	items := make([]session.PlanItem, 0, n)
	for i := range n {
		items = append(items, session.PlanItem{
			ID:      fmt.Sprintf("s%d", i+1),
			Content: fmt.Sprintf("step number %d", i+1),
			Status:  session.PlanPending,
			Type:    session.StepEdit,
		})
	}
	return session.Plan{Revision: rev, Items: items}
}

func scrollPlanToBottom(t *testing.T, s *Sidebar) {
	t.Helper()
	for range 200 {
		s.HandleScrollKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyDown, Mods: xui.ModCtrl})
	}
}

// Scrolling to the bottom must reveal the last plan line: the hint row
// reserves a viewport line, so the clamp has to use the rendered view
// height, not the full plan pane height.
func TestPlanBottomScrollShowsLastStep(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	s.SetPlan(numberedPlan(20, 1))
	_ = drawText(s, 26)
	scrollPlanToBottom(t, s)

	txt := drawText(s, 26)
	require.Contains(t, txt, "step number 20",
		"the last plan line must be visible after scrolling to the bottom")
}

// An operational update (a step status change) bumps the plan revision
// without changing the plan itself; the viewport the user scrolled to must
// survive it. A material edit to the plan content still resets the view.
func TestPlanScrollSurvivesOperationalRevisionBumps(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 128000)
	s.Toggle()
	plan := numberedPlan(20, 1)
	s.SetPlan(plan)
	_ = drawText(s, 26)
	scrollPlanToBottom(t, s)
	before := s.planScroll
	require.Positive(t, before, "the fixture must scroll somewhere")

	plan.Revision = 2
	plan.Items[3].Status = session.PlanCompleted
	s.SetPlan(plan)
	_ = drawText(s, 26)
	require.Equal(t, before, s.planScroll,
		"a status-only revision bump must not move the viewport")

	// Material edits arrive with a bumped contract epoch, exactly as the
	// session persists them.
	plan.Revision = 3
	plan.ContractEpoch = 2
	plan.Items[7].Content = "step number 8 (edited)"
	s.SetPlan(plan)
	_ = drawText(s, 26)
	require.Zero(t, s.planScroll,
		"a material plan edit must reset the viewport")
}

// skillPlan builds a one-step plan carrying two skills — tdd on, grill off —
// so circle-state assertions have both sides of the toggle in one fixture.
func skillPlan(approved bool) session.Plan {
	return session.Plan{
		Revision: 7, Approved: approved, Goal: "ship the fix",
		Items: []session.PlanItem{{
			ID: "s1", Content: "edit the code", Status: session.PlanPending, Type: session.StepEdit,
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
				Skills: []string{"tdd", "grill"}, DisabledSkills: []string{"grill"},
			}},
		}},
	}
}

// skillLineStyle finds the rendered row of one named skill and returns its
// style, failing the test when the row is missing or duplicated.
func skillLineStyle(t *testing.T, s *Sidebar, name string) (string, xui.Style) {
	t.Helper()
	lines, _ := s.planContent(contentWidth(s.CurrentWidth()), xui.WidthUnicode)
	var row string
	var style xui.Style
	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line.text), name) {
			require.Empty(t, row, "skill %s rendered twice", name)
			row, style = line.text, line.style
		}
	}
	require.NotEmpty(t, row, "skill %s must render its own row", name)
	return row, style
}

func TestSidebarStepSkillsRenderFourCircleStates(t *testing.T) {
	theme := components.DefaultTheme()

	// Draft: an on skill is filled, an off one is hollow and muted — the plan
	// is not in force, so nothing is a promise yet.
	s := visiblePlanSidebar(t)
	s.SetPlan(skillPlan(false))
	onRow, onStyle := skillLineStyle(t, s, "tdd")
	offRow, offStyle := skillLineStyle(t, s, "grill")
	assert.Contains(t, onRow, "●", "a live draft skill is a filled circle")
	assert.Equal(t, theme.Foreground, onStyle)
	assert.Contains(t, offRow, "○", "an off skill is a hollow circle")
	assert.Equal(t, theme.Muted, offStyle)

	// Approved, never run: the on skill turns into a hollow green promise.
	s.SetPlan(skillPlan(true))
	onRow, onStyle = skillLineStyle(t, s, "tdd")
	assert.Contains(t, onRow, "○", "an approved not-yet-run skill is hollow")
	assert.Equal(t, theme.Success, onStyle)

	// Approved with a clean run: the promise fills green.
	plan := skillPlan(true)
	plan.Items[0].Actions[0].Runs = []session.PlanActionRun{{Status: session.PlanActionRunOK}}
	s.SetPlan(plan)
	onRow, onStyle = skillLineStyle(t, s, "tdd")
	assert.Contains(t, onRow, "●", "a cleanly run skill is filled")
	assert.Equal(t, theme.Success, onStyle)
}

// rowContaining finds the surface row whose text holds the marker, so click
// tests derive their Y from what was drawn, not from the hit tables.
func rowContaining(t *testing.T, text, marker string) int {
	t.Helper()
	for row, line := range strings.Split(text, "\n") {
		if strings.Contains(line, marker) {
			return row
		}
	}
	t.Fatalf("no drawn row contains %q", marker)
	return -1
}

func TestSidebarSkillClickTogglesThroughCallback(t *testing.T) {
	s := visiblePlanSidebar(t)
	s.SetPlan(skillPlan(true))
	var gotStep, gotSkill string
	var gotAction int
	var gotDisabled bool
	calls := 0
	s.ConfigureSkillToggle(func(stepID string, actionIndex int, skill string, disabled bool) error {
		calls++
		gotStep, gotAction, gotSkill, gotDisabled = stepID, actionIndex, skill, disabled
		return nil
	})

	text := drawWide(s, 56, 40)

	// Clicking the on skill asks to disable it.
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 4,
		Y: rowContaining(t, text, "○ tdd"),
	})
	assert.True(t, ctx.Consume, "a skill-row click belongs to the sidebar")
	require.Equal(t, 1, calls)
	assert.Equal(t, "s1", gotStep)
	assert.Equal(t, 0, gotAction, "the toggle addresses the step's inject_skill action")
	assert.Equal(t, "tdd", gotSkill)
	assert.True(t, gotDisabled, "an on skill toggles toward off")

	// Clicking the off skill asks to enable it again.
	s.Handle(ctx, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 4,
		Y: rowContaining(t, text, "○ grill"),
	})
	require.Equal(t, 2, calls)
	assert.Equal(t, "grill", gotSkill)
	assert.False(t, gotDisabled, "an off skill toggles toward on")
}

func TestSidebarSkillClickWithoutStepIDSelectsInstead(t *testing.T) {
	s := visiblePlanSidebar(t)
	plan := skillPlan(true)
	plan.Items[0].ID = "" // a legacy item cannot route a toggle — no id, no callback
	s.SetPlan(plan)
	calls := 0
	s.ConfigureSkillToggle(func(string, int, string, bool) error {
		calls++
		return nil
	})

	text := drawWide(s, 56, 40)
	ctx := &components.EventContext{}
	s.Handle(ctx, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: 4,
		Y: rowContaining(t, text, "○ tdd"),
	})
	assert.True(t, ctx.Consume)
	assert.Zero(t, calls, "a skill without a routable step id must not fire the toggle")
	assert.True(t, s.planFocus, "the click still selects the owning step")
}
