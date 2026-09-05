package sidebar

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// clickChip presses a stepper chip where Draw painted it.
func clickChip(s *Sidebar, x, y int) {
	s.Handle(&components.EventContext{}, xui.MouseEvent{
		Action: xui.MousePress, Button: xui.MouseLeft, X: x, Y: y,
	})
}

// TestSidebarContextSteppers pins the session-only context rows: they render
// on the settings tab with ⊖/⊕ chips flanking the value, each click steps
// 10k from a 50k start, stepping below
// the 10k floor resets to the General default, and the controller callback
// receives the stepped value immediately — no keyboard entry anywhere.
func TestSidebarContextSteppers(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 0)
	s.Toggle()
	s.setTab(tabSettings)

	var gotMain, gotAgents []int
	s.ConfigureContext(150_000, 100_000, func(tokens int) {
		gotMain = append(gotMain, tokens)
	}, func(tokens int) {
		gotAgents = append(gotAgents, tokens)
	})

	text := drawText(s, 24)
	require.Contains(t, text, "compact 150k", "main row shows the General default")
	require.Contains(t, text, "agents 100k")
	require.Contains(t, text, "⊖")
	require.Contains(t, text, "⊕")

	// + steps the main row by 10k; the display setter simulates the view
	// pushing the controller's answer back.
	clickChip(s, s.mainPlusX, s.mainCtxRowY)
	require.Equal(t, []int{160_000}, gotMain)
	s.SetReminderThreshold(160_000)
	assert.Contains(t, drawText(s, 24), "compact 160k")

	// Repeated clicks keep stepping, each with the view's push-back.
	clickChip(s, s.mainPlusX, s.mainCtxRowY)
	s.SetReminderThreshold(170_000)
	clickChip(s, s.mainPlusX, s.mainCtxRowY)
	require.Equal(t, []int{170_000, 180_000}, gotMain[1:])
	s.SetReminderThreshold(180_000)

	// − steps down; from 10k the next − would land at 0, below the floor —
	// the reset hands 0 to the callback and the view pushes the restored
	// General value back into the display.
	s.SetReminderThreshold(20_000)
	clickChip(s, s.mainMinusX, s.mainCtxRowY)
	require.Equal(t, 10_000, gotMain[len(gotMain)-1])
	s.SetReminderThreshold(10_000)
	clickChip(s, s.mainMinusX, s.mainCtxRowY)
	require.Zero(t, gotMain[len(gotMain)-1], "one step below the floor resets to the General default")
	s.SetReminderThreshold(150_000)
	assert.Contains(t, drawText(s, 24), "compact 150k")

	// The agents row is independent: its chips step its own value.
	clickChip(s, s.agentsPlusX, s.agentsCtxRowY)
	require.Equal(t, []int{110_000}, gotAgents)
	s.SetAgentsContext(110_000)
	assert.Contains(t, drawText(s, 24), "agents 110k")
	assert.Contains(t, drawText(s, 24), "compact 150k", "the main row survives next to it")

	// A miss between the chips does nothing.
	before := len(gotMain)
	clickChip(s, s.mainMinusX+3, s.mainCtxRowY)
	assert.Len(t, gotMain, before, "the gap between chips is inert")
}

// TestSidebarContextDefaults: a zero General value renders as the muted
// default/unlimited labels, and the first − resets to that default (0 →
// unlimited semantics live in the controller, not here).
func TestSidebarContextDefaults(t *testing.T) {
	s := NewSidebar(components.DefaultTheme(), 0)
	s.Toggle()
	s.setTab(tabSettings)

	var gotMain, gotAgents int
	s.ConfigureContext(0, 0, func(tokens int) {
		gotMain = tokens
	}, func(tokens int) {
		gotAgents = tokens
	})

	text := drawText(s, 24)
	require.Contains(t, text, "compact default")
	require.Contains(t, text, "agents ∞")

	// From 0 the first + lands on the 50k start; a step down stays above the
	// floor, and only a step below 10k resets to 0.
	clickChip(s, s.mainPlusX, s.mainCtxRowY)
	assert.Equal(t, 50_000, gotMain)
	s.SetReminderThreshold(50_000)
	clickChip(s, s.mainMinusX, s.mainCtxRowY)
	assert.Equal(t, 40_000, gotMain)
	s.SetReminderThreshold(10_000)
	clickChip(s, s.mainMinusX, s.mainCtxRowY)
	assert.Zero(t, gotMain)
	s.SetReminderThreshold(0)
	assert.Contains(t, drawText(s, 24), "compact default")

	clickChip(s, s.agentsPlusX, s.agentsCtxRowY)
	assert.Equal(t, 50_000, gotAgents)
	s.SetAgentsContext(50_000)
	clickChip(s, s.agentsMinusX, s.agentsCtxRowY)
	assert.Equal(t, 40_000, gotAgents)
	s.SetAgentsContext(10_000)
	clickChip(s, s.agentsMinusX, s.agentsCtxRowY)
	assert.Zero(t, gotAgents)
	s.SetAgentsContext(0)
	assert.Contains(t, drawText(s, 24), "agents ∞")
}
