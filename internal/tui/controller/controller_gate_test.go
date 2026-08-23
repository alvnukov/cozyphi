package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/permission"
)

func TestInitGateDefaultAsks(t *testing.T) {
	c := &Controller{}
	c.initGate(permission.DefaultPolicy())
	assert.False(t, c.AllowAll())

	dec, _ := c.gate.Check(t.Context(), permission.Request{
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

	dec, _ := c.gate.Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "rm -rf /",
	})
	assert.Equal(t, permission.Allow, dec)
}
