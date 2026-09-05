package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAgentWindowLimit pins the spawn-time ceiling: the smaller of the
// persisted agents.context_limit and this session's override wins, and zero
// means unlimited — no limit, no override, nothing applied.
func TestAgentWindowLimit(t *testing.T) {
	c := &Controller{}

	assert.Zero(t, c.AgentWindowLimit(), "fresh controller: unlimited")

	c.SetAgentContextLimit(32000)
	assert.Equal(t, 32000, c.AgentWindowLimit())

	c.SetSessionAgentContext(16000)
	assert.Equal(t, 16000, c.AgentWindowLimit(), "a narrower session override wins")

	c.SetSessionAgentContext(64000)
	assert.Equal(t, 32000, c.AgentWindowLimit(), "a wider session override cannot beat the persisted limit")

	c.SetSessionAgentContext(0)
	assert.Equal(t, 32000, c.AgentWindowLimit(), "zero override restores the persisted limit")

	c.SetAgentContextLimit(-1)
	assert.Equal(t, 0, c.AgentWindowLimit(), "negative input is clamped: no limit again")
}
