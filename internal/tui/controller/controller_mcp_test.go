package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/mcp"
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

// TestToggleMCPServerPersistsDisabledList covers the whole stack behind the
// toggle: pool state now, user-config persistence for the next session.
func TestToggleMCPServerPersistsDisabledList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	c := &Controller{mcpPool: mcp.NewPool(map[string]mcp.ServerConfig{
		"alpha": {},
		"beta":  {},
	})}

	require.NoError(t, c.ToggleMCPServer("beta", false))
	assert.Equal(t, []string{"alpha"}, c.MCPServers(), "disabled server disappears for the model")

	data, err := os.ReadFile(filepath.Join(home, ".cozyphi", "mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"disabled"`)
	assert.Contains(t, string(data), `"beta"`)

	require.NoError(t, c.ToggleMCPServer("beta", true))
	assert.Empty(t, c.mcpPool.DisabledNames(), "re-enable clears the persisted choice")
	assert.Equal(t, []string{"alpha", "beta"}, c.MCPServers())

	require.ErrorContains(t, c.ToggleMCPServer("ghost", false), `unknown mcp server "ghost"`)

	var none Controller
	require.ErrorContains(t, none.ToggleMCPServer("alpha", false), "mcp is not available")
}
