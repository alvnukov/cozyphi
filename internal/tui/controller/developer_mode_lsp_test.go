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

// developerLSPRuntime starts a granted developer session over a workspace the
// product opened by its own rules, with one executable named gopls on PATH so
// the install lookup has something real to find. The directory holding it is a
// sentinel: it is the resolved path, and no answer may carry it.
func developerLSPRuntime(t *testing.T) (c *Controller, cwd, sentinel string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")

	sentinel = filepath.Join(t.TempDir(), "SENTINEL-bin")
	require.NoError(t, os.MkdirAll(sentinel, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sentinel, "gopls"), []byte("#!/bin/sh\nexit 1\n"), 0o700))
	t.Setenv("PATH", sentinel)

	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(twoModelConfig("alpha")), 0o600))

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err = rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, cwd, sentinel
}

func lspField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryIntegrations, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessReportsTheLanguageServersThisWorkspaceCouldRun(t *testing.T) {
	c, cwd, _ := developerLSPRuntime(t)
	require.NotNil(t, c.lspMgr, "the workspace opened a manager without starting anything")

	state := lspField(t, c, diag.KeyLSPState)
	assert.Equal(t, string(diag.LSPEnabled), state.Configured.Value.Str)
	assert.Equal(t, string(diag.LSPOpened), state.Loaded.Value.Str)
	assert.Equal(t, string(diag.LSPIdle), state.Effective.Value.Str,
		"the server is on this machine and nothing has needed it yet")

	assert.Equal(t, cwd, lspField(t, c, diag.KeyLSPWorkspace).Effective.Value.Str,
		"a server would be started for the workspace this session opened")

	languages := lspField(t, c, diag.KeyLSPLanguages)
	assert.Equal(t, []string{"go"}, languages.Configured.Value.List,
		"this build carries one language server, whatever else is installed")
	assert.Equal(t, []string{"go"}, languages.Loaded.Value.List)
	assert.Empty(t, languages.Effective.Value.List, "nothing is running")

	servers := lspField(t, c, diag.KeyLSPServers)
	assert.Equal(t, []string{"gopls"}, servers.Configured.Value.List)
	assert.Equal(t, []string{"gopls"}, servers.Loaded.Value.List, "the lookup found one on PATH")
	assert.Empty(t, servers.Effective.Value.List)

	operations := lspField(t, c, diag.KeyLSPOperations)
	assert.Equal(t,
		[]string{
			"definition", "references", "implementations", "type_definition",
			"hover", "symbols", "calls", "diagnostics",
		},
		operations.Configured.Value.List)
	assert.Equal(t, diag.StateUnavailable, operations.Effective.State,
		"what a running server would accept is a question only a query answers")

	assert.Empty(t, lspField(t, c, diag.KeyLSPRoots).Effective.Value.List)
	assert.Equal(t, string(diag.LSPStartNotAttempted), lspField(t, c, diag.KeyLSPStart).Effective.Value.Str)

	diagnostics := lspField(t, c, diag.KeyLSPDiagnostics)
	assert.Equal(t, []string{"fresh", "cached", "unconfirmed", "pending"}, diagnostics.Configured.Value.List)
	assert.Equal(t, diag.StateUnavailable, diagnostics.Effective.State,
		"a freshness belongs to the query that fetched it, and this view runs none")
}

// The question must be free to ask. If reading the category started gopls, a
// developer looking at the harness would spawn a language server by looking at
// a list — and would change the answer by reading it.
func TestAskingTheHarnessAboutLSPStartsNoLanguageServer(t *testing.T) {
	c, _, sentinel := developerLSPRuntime(t)

	for range 3 {
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
		require.NoError(t, err)
		_, err = c.diagnostics.Snapshot(t.Context(), "")
		require.NoError(t, err)
		require.NotEmpty(t, c.diagnostics.Catalog().Categories)
		for _, key := range []string{diag.KeyLSPState, diag.KeyLSPServers, diag.KeyLSPRoots} {
			lspField(t, c, key)
		}
	}

	assert.Equal(t, string(diag.LSPStartNotAttempted), lspField(t, c, diag.KeyLSPStart).Effective.Value.Str,
		"three rounds of questions and still nothing has asked for a server")
	assert.Empty(t, lspField(t, c, diag.KeyLSPRoots).Effective.Value.List)
	assert.Equal(t, string(diag.LSPIdle), lspField(t, c, diag.KeyLSPState).Effective.Value.Str)

	// The resolved executable is what the lookup found, and it is the one
	// thing about it that must not travel: the path can name a directory the
	// owner chose.
	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	for _, field := range snapshot.Categories[0].Fields {
		for _, observation := range []diag.Observation{field.Configured, field.Loaded, field.Effective} {
			assert.NotContains(t, observation.Value.Str, sentinel, field.Key)
			assert.NotContains(t, observation.Source.Ref, sentinel, field.Key)
			for _, item := range observation.Value.List {
				assert.NotContains(t, item, sentinel, field.Key)
			}
		}
	}
}

// With nothing on the machine to run, the answer is a missing binary rather
// than a broken subsystem — and the two are fixed in entirely different places.
func TestAWorkspaceWithNoInstalledServerSaysSoRatherThanFailing(t *testing.T) {
	c, _, _ := developerLSPRuntime(t)
	t.Setenv("PATH", t.TempDir())

	state := lspField(t, c, diag.KeyLSPState)
	assert.Equal(t, string(diag.LSPEnabled), state.Configured.Value.Str,
		"nothing switched it off; there is simply nothing to run")
	assert.Equal(t, string(diag.LSPNotInstalled), state.Effective.Value.Str)

	servers := lspField(t, c, diag.KeyLSPServers)
	assert.Equal(t, []string{"gopls"}, servers.Configured.Value.List,
		"the build still carries the profile, which is why the gap is legible")
	assert.Empty(t, servers.Loaded.Value.List)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), "")
	require.NoError(t, err)
	answered := map[diag.Category]diag.Availability{}
	for _, category := range snapshot.Categories {
		answered[category.Category] = category.Availability
	}
	assert.Equal(t, diag.AvailabilityAvailable, answered[diag.CategoryIntegrations],
		"a server that is not installed is an answer, not an unavailable category")
	assert.Equal(t, diag.AvailabilityAvailable, answered[diag.CategoryRuntime])
}

// Closing the workspace shuts the manager down, and the harness must say so:
// after that, no query will start a server again.
func TestTheHarnessSaysWhenTheManagerHasBeenShutDown(t *testing.T) {
	c, _, _ := developerLSPRuntime(t)
	require.NoError(t, c.lspMgr.Close(t.Context()))

	state := lspField(t, c, diag.KeyLSPState)
	assert.Equal(t, string(diag.LSPOpened), state.Loaded.Value.Str,
		"it did exist, and that is why there is something to report")
	assert.Equal(t, string(diag.LSPClosed), state.Effective.Value.Str)
	assert.Empty(t, lspField(t, c, diag.KeyLSPRoots).Effective.Value.List)
}
