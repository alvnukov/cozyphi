package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

// newThresholdController stands a controller on a known model window, so the
// window-derived fallback in ReminderThreshold is a fixed number the tests
// can name. The engine stays nil: applyReminder is nil-guarded, and the
// engine-side effect of SetCompactionSettings has its own test in the agent
// package (TestSetCompactionSettingsMovesReminderThreshold).
func newThresholdController() *Controller {
	c := &Controller{}
	c.modelCfg = llm.ModelConfig{ContextWindow: 200_000}
	return c
}

// TestAgentWindowLimit pins the spawn-time ceiling: the session override
// replaces the General value for this session (General is the default, not a
// cap), zero restores the General value, and nothing at all means unlimited.
func TestAgentWindowLimit(t *testing.T) {
	c := &Controller{}

	assert.Zero(t, c.AgentWindowLimit(), "fresh controller: unlimited")

	c.SetAgentContextLimit(100_000)
	assert.Equal(t, 100_000, c.AgentWindowLimit(), "no override: the General value applies")

	c.SetSessionAgentContext(150_000)
	assert.Equal(t, 150_000, c.AgentWindowLimit(), "a session override may exceed the General value")

	c.SetSessionAgentContext(0)
	assert.Equal(t, 100_000, c.AgentWindowLimit(), "zero override restores the General value")

	c.SetAgentContextLimit(0)
	assert.Zero(t, c.AgentWindowLimit(), "General 0 with no override: unlimited again")

	c.SetSessionAgentContext(-1)
	assert.Zero(t, c.AgentWindowLimit(), "negative input is clamped: still unlimited")
}

// TestReminderThresholdSessionOverride: the General value is the fallback; a
// session override replaces it in both directions (past the General value
// too); clearing the override returns to the General value; General 0 falls
// back to the window-derived default.
func TestReminderThresholdSessionOverride(t *testing.T) {
	c := newThresholdController()

	derived := compaction.ConfiguredSettings(0).ReminderThreshold(200_000)
	assert.NotZero(t, derived, "a known window must derive a default")
	assert.Equal(t, derived, c.ReminderThreshold(), "no General value yet: window-derived default")

	c.SetReminderThreshold(150_000)
	assert.Equal(t, 150_000, c.ReminderThreshold())

	c.SetSessionReminderThreshold(200_000)
	assert.Equal(t, 200_000, c.ReminderThreshold(), "the override replaces the General value")

	c.SetSessionReminderThreshold(100_000)
	assert.Equal(t, 100_000, c.ReminderThreshold(), "the override may also sit below the General value")

	c.SetSessionReminderThreshold(0)
	assert.Equal(t, 150_000, c.ReminderThreshold(), "clearing returns to the General value, not the derived one")

	c.SetReminderThreshold(0)
	assert.Equal(t, derived, c.ReminderThreshold(), "General 0: window-derived default again")
}

// TestSetReminderThresholdKeepsOverride: applying a new General value while a
// session override is active must not clobber the override — the pane may
// republish its snapshot on every apply.
func TestSetReminderThresholdKeepsOverride(t *testing.T) {
	c := newThresholdController()

	c.SetReminderThreshold(150_000)
	c.SetSessionReminderThreshold(250_000)

	c.SetReminderThreshold(100_000)
	assert.Equal(t, 250_000, c.ReminderThreshold(), "the live override survives a General republish")

	c.SetSessionReminderThreshold(0)
	assert.Equal(t, 100_000, c.ReminderThreshold(), "clearing later picks up the newest General value")
}
