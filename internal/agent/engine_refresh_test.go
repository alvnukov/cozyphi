package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
)

// RefreshTools must re-read the MCP catalog from the shared pool: a server
// disabled after startup leaves the system prompt without a new engine.
func TestRefreshToolsDropsDisabledServerFromPrompt(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{
		"alpha": {Command: []string{"true"}},
		"beta":  {Command: []string{"true"}},
	})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "test", APIKey: "x", BaseURL: "http://127.0.0.1:9"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		MCP:         pool,
	})
	require.NoError(t, err)

	before := engine.systemPrompt()
	require.Contains(t, before, "alpha")
	require.Contains(t, before, "beta")

	require.NoError(t, pool.SetEnabled("beta", false))
	engine.RefreshTools()

	after := engine.systemPrompt()
	require.Contains(t, after, "alpha")
	require.NotContains(t, after, "beta", "disabled server must leave the prompt catalog")
}
