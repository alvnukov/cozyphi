package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/mcp"
)

// TestMCPServersSortedNames pins the sidebar data source: configured server
// names, sorted, nil-safe without a pool.
func TestMCPServersSortedNames(t *testing.T) {
	var c Controller
	assert.Nil(t, c.MCPServers(), "nil pool is safe")

	c.mcpPool = mcp.NewPool(map[string]mcp.ServerConfig{
		"zeta":  {},
		"alpha": {},
	})
	assert.Equal(t, []string{"alpha", "zeta"}, c.MCPServers())
}
