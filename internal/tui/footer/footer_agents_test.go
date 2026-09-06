package footer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// agentFooter pairs a chrome with a fixed live-child count, the way the view
// wires it to controller.LiveJobCount.
func agentFooter(n int) *FooterChrome {
	f := NewFooterChrome(components.DefaultTheme(), 0)
	f.SetLiveJobs(func() int { return n })
	return f
}

// The footer calls a running child what every other surface calls it: an
// agent. The count keeps its singular and its plural.
func TestQuietFooterCountsLiveAgents(t *testing.T) {
	assert.Contains(t, drawRow(agentFooter(1), 80), "1 agent")
	assert.Contains(t, drawRow(agentFooter(3), 80), "3 agents")
}

// No live child, no label — and a footer nobody wired says nothing either.
func TestQuietFooterHidesTheAgentLabelWhenNoneIsLive(t *testing.T) {
	assert.NotContains(t, drawRow(agentFooter(0), 80), "agent")

	bare := NewFooterChrome(components.DefaultTheme(), 0)
	assert.NotContains(t, drawRow(bare, 80), "agent")
}

// The streaming footer carries the same label, so a child stays visible while
// the parent works.
func TestLiveFooterCountsLiveAgents(t *testing.T) {
	f := agentFooter(2)
	snap := liveSnap()
	f.SetLabelContext(func() session.Snapshot { return snap })
	f.Activity().Apply(controller.ActivityStreaming)

	assert.Contains(t, drawRow(f, 120), "2 agents")
}
