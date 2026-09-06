package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

const (
	// globalMCPConfig switches one of its own servers off, so the harness has
	// a persisted choice to tell from a session's.
	globalMCPConfig = `{
	  "servers": {
	    "shared": {"command": ["true"], "args": ["--from-global"]},
	    "global-only": {"command": ["true"]}
	  },
	  "disabled": ["global-only"]
	}`
	projectMCPConfig = `{
	  "servers": {
	    "shared": {"command": ["true"], "args": ["--from-project"]},
	    "project-only": {"command": ["true"]}
	  }
	}`
)

// developerMCPRuntime starts a granted developer session over two real mcp.json
// files, written before the workspace loads its pool. The harness then reports
// a pool the product built by its own rules rather than one a test handed it.
func developerMCPRuntime(t *testing.T, mode string) (*Controller, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	t.Setenv("COZYPHI_MCP", mode)
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(twoModelConfig("alpha")), 0o600))

	writeConfig(t, filepath.Join(home, ".cozyphi", "mcp.json"), globalMCPConfig)
	writeConfig(t, proj.MCPConfigFile(), projectMCPConfig)

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, cwd
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func mcpField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryIntegrations, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessReportsTheMCPPoolThisWorkspaceActuallyHolds(t *testing.T) {
	c, cwd := developerMCPRuntime(t, "on")

	state := mcpField(t, c, diag.KeyMCPState)
	assert.Equal(t, string(diag.MCPEnabled), state.Configured.Value.Str)
	assert.Equal(t, string(diag.MCPReady), state.Effective.Value.Str)

	servers := mcpField(t, c, diag.KeyMCPServers)
	assert.Equal(t, []string{"global-only", "project-only", "shared"}, servers.Configured.Value.List,
		"both files were merged, and the name they share is one server")
	assert.Equal(t, []string{"project-only", "shared"}, servers.Loaded.Value.List,
		"the global file switched one off before the session began")
	assert.Empty(t, servers.Effective.Value.List,
		"nothing has called a server, so nothing is connected")

	global := mcpField(t, c, diag.KeyMCPGlobal)
	assert.Equal(t, []string{"global-only", "shared"}, global.Configured.Value.List)
	assert.Equal(t, []string{"global-only"}, global.Loaded.Value.List,
		"the project file redefined the shared name, so the global one did not supply it")

	workspace := mcpField(t, c, diag.KeyMCPProject)
	assert.Equal(t, []string{"project-only", "shared"}, workspace.Configured.Value.List)
	assert.Equal(t, []string{"project-only", "shared"}, workspace.Loaded.Value.List)

	assert.Empty(t, mcpField(t, c, diag.KeyMCPImported).Configured.Value.List,
		"nothing imported another tool's configuration here")
	assert.Equal(t, []string{"global-only"}, mcpField(t, c, diag.KeyMCPDisabled).Configured.Value.List)
	assert.Empty(t, mcpField(t, c, diag.KeyMCPFailed).Effective.Value.List)
	assert.Equal(t, cwd, mcpField(t, c, diag.KeyMCPWorkspace).Effective.Value.Str,
		"a local server would be spawned in the workspace this session opened")
}

// The question must be free to ask. If reading the category connected to a
// server, a developer looking at the harness would change what the harness
// reports — and would spawn processes by looking at a list.
func TestAskingTheHarnessAboutMCPStartsNoServer(t *testing.T) {
	c, _ := developerMCPRuntime(t, "on")
	pool := c.mcpPool
	require.NotNil(t, pool)

	before := pool.ServerStatuses()
	names := pool.ServerNames()

	for range 3 {
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
		require.NoError(t, err)
		_, err = c.diagnostics.Snapshot(t.Context(), "")
		require.NoError(t, err)
		require.NotEmpty(t, c.diagnostics.Catalog().Categories)
		for _, key := range []string{diag.KeyMCPState, diag.KeyMCPServers, diag.KeyMCPFailed} {
			mcpField(t, c, key)
		}
	}

	assert.Equal(t, before, pool.ServerStatuses(),
		"every server stands exactly where it stood before the question")
	assert.Equal(t, names, pool.ServerNames())
	for _, status := range pool.ServerStatuses() {
		assert.NotEqual(t, "connected", string(status.State), status.Name)
	}
}

// A session whose workspace has no pool must still answer everything else. A
// missing owner is one honest category, not a broken harness.
func TestAWorkspaceWithNoPoolAnswersWithoutInventingOne(t *testing.T) {
	c, _ := developerMCPRuntime(t, "off")
	require.Nil(t, c.mcpPool, "the switch is honored by the loader, so no pool exists to describe")

	state := mcpField(t, c, diag.KeyMCPState)
	assert.Equal(t, string(diag.MCPDisabled), state.Configured.Value.Str)
	assert.Equal(t, "COZYPHI_MCP", state.Configured.Source.Ref)
	assert.Equal(t, string(diag.MCPDisabled), state.Effective.Value.Str)

	servers := mcpField(t, c, diag.KeyMCPServers)
	assert.Equal(t, diag.StateNotApplicable, servers.Effective.State,
		"the two configured files are not reported as servers that exist but are unreachable")
	assert.Empty(t, servers.Effective.Value.List)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), "")
	require.NoError(t, err)
	answered := map[diag.Category]diag.Availability{}
	for _, category := range snapshot.Categories {
		answered[category.Category] = category.Availability
	}
	assert.Equal(t, diag.AvailabilityAvailable, answered[diag.CategoryIntegrations],
		"a subsystem switched off is an answer, not an unavailable category")
	assert.Equal(t, diag.AvailabilityAvailable, answered[diag.CategoryRuntime],
		"and the rest of the harness answers as it always did")
	assert.Equal(t, diag.AvailabilityAvailable, answered[diag.CategoryTools])
}
