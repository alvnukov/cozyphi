package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

// awaitAsk drains a controller's bus until it publishes a permission ask.
func awaitAsk(t *testing.T, c *Controller) PermissionAskMsg {
	t.Helper()
	var ask PermissionAskMsg
	waitForCond(t, 5*time.Second, func() bool {
		for _, m := range c.bus.Drain() {
			if request, ok := m.(PermissionAskMsg); ok {
				ask = request
			}
		}
		return ask.Reply != nil
	})
	return ask
}

// TestSessionGrantFromAChildStaysInThatChild: "Allow all for this session"
// answered on a sub-agent's ask is that sub-agent's grant. The session the
// user is sitting in keeps asking, because it never asked for this.
func TestSessionGrantFromAChildStaysInThatChild(t *testing.T) {
	parent := newReadyController(t)
	defer parent.Cancel()
	child := newReadyController(t)
	defer child.Cancel()

	done := make(chan permission.AskResult, 1)
	go func() {
		r, err := child.askPermission(t.Context(), permission.Request{
			Tool: "bash", Action: permission.ActionBash, Command: "ls",
		}, "")
		require.NoError(t, err)
		done <- r
	}()

	ask := awaitAsk(t, child)
	ask.Reply <- AskReply{Approved: true, AllowSession: true}
	select {
	case r := <-done:
		assert.True(t, r.Approved)
	case <-time.After(5 * time.Second):
		t.Fatal("the answer must reach the session that asked")
	}

	assert.True(t, child.AllowAll(), "the grant binds to the session that asked")
	assert.False(t, parent.AllowAll(), "a sub-agent cannot grant for its parent")
	for _, m := range parent.bus.Drain() {
		if _, wrong := m.(PermissionAskMsg); wrong {
			t.Fatal("a child's ask must not appear on its parent's bus")
		}
	}
}

// TestAnsweredAskStopsSayingItIsWaiting: the answer resolves the overlay on
// whichever screen showed it, so the session that asked hears nothing more
// unless the ask is withdrawn. A sub-agent's row would keep its ⏸ mark for
// the rest of the turn, so the withdrawal is published on the way out.
func TestAnsweredAskStopsSayingItIsWaiting(t *testing.T) {
	c := newReadyController(t)
	defer c.Cancel()

	go func() {
		_, err := c.askPermission(t.Context(), permission.Request{
			Tool: "bash", Action: permission.ActionBash, Command: "ls",
		}, "")
		require.NoError(t, err)
	}()

	ask := awaitAsk(t, c)
	ask.Reply <- AskReply{Approved: true}

	var dismissed bool
	waitForCond(t, 5*time.Second, func() bool {
		for _, m := range c.bus.Drain() {
			if _, ok := m.(PermissionDismissMsg); ok {
				dismissed = true
			}
		}
		return dismissed
	})
}
