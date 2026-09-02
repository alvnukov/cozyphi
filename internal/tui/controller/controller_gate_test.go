package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestInitGateDefaultAsks(t *testing.T) {
	c := &Controller{}
	c.initGate(permission.DefaultPolicy())
	assert.False(t, c.AllowAll())

	dec, _ := c.currentGate().Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "pip install numpy",
	})
	assert.Equal(t, permission.Ask, dec)
}

func TestInitGateDangerouslyAllowAllBypasses(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.DangerouslyAllowAll = true
	c := &Controller{}
	c.initGate(policy)
	assert.True(t, c.AllowAll())

	dec, _ := c.currentGate().Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "rm -rf /",
	})
	assert.Equal(t, permission.Allow, dec)
}

// Every assembly the controller can install must be complete, and a
// reconfiguration (SetModel re-runs initGate) must never leave a window
// where the boundary is missing or permits an outside-workspace write.
func TestInitGateReconfigurationStaysCompleteAndClosed(t *testing.T) {
	c := &Controller{}

	for i := range 5 {
		c.initGate(permission.DefaultPolicy())
		bypass, ok := c.currentGate().(*permission.BypassGate)
		require.True(t, ok, "the controller must install a BypassGate")
		require.NotNil(t, bypass.Inner, "the inner gate must always be installed")
		require.NotNil(t, bypass.Enabled, "the bypass toggle must always be wired")

		dec, _ := c.currentGate().Check(t.Context(), permission.Request{
			Action: permission.ActionWrite,
			Tool:   "write",
			Paths:  []string{"/definitely/outside/f.txt"},
		})
		assert.Equal(t, permission.Deny, dec, "re-init %d: an outside-workspace write must stay denied", i)
	}
}
