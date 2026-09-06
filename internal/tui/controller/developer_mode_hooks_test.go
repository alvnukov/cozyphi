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

// writeHooks writes one hook directory whose script, if anything ever ran it,
// would say so. The marker is the proof the harness view fires nothing, and
// the directory name is a sentinel so a leaked path is recognizable anywhere
// it appears.
func writeHooks(t *testing.T, dir, marker, manifest string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644))
	script := "#!/bin/sh\necho ran >> " + marker + "\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o700))
}

// developerHooksRuntime starts a granted developer session over a workspace
// with hooks in both directories, one name defined in both — which is the only
// arrangement where precedence is observable at all.
func developerHooksRuntime(t *testing.T) (c *Controller, proj *project.Project, marker string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	t.Setenv("COZYPHI_HOOKS", "")

	marker = filepath.Join(t.TempDir(), "SENTINEL-hook-ran")
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj, err = project.Discover(cwd)
	require.NoError(t, err)

	configFile := proj.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(twoModelConfig("alpha")), 0o600))

	writeHooks(t, proj.Global().HooksDir(), marker, `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","fail_closed":true,"timeout":"9s"},
	  {"name":"audit","event":"post_tool","run":"./run.sh","async":true}
	]}`)
	writeHooks(t, proj.HooksDir(), marker, `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","timeout":"12s"}
	]}`)

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err = rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c, proj, marker
}

func hooksField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryIntegrations, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessReportsTheHooksThisSessionRunsUnder(t *testing.T) {
	c, _, _ := developerHooksRuntime(t)
	require.NotNil(t, c.Hooks(), "the workspace loaded a manager without running anything")

	state := hooksField(t, c, diag.KeyHooksState)
	assert.Equal(t, string(diag.HooksEnabled), state.Configured.Value.Str)
	assert.Equal(t, string(diag.HooksLoaded), state.Loaded.Value.Str)
	assert.Equal(t, string(diag.HooksActive), state.Effective.Value.Str,
		"hooks are loaded and nothing has triggered one")

	registered := hooksField(t, c, diag.KeyHooksRegistered)
	assert.Equal(t, []string{"audit", "guard-bash"}, registered.Configured.Value.List)
	assert.Equal(t, []string{"audit", "guard-bash"}, registered.Effective.Value.List)

	user := hooksField(t, c, diag.KeyHooksUser)
	assert.Equal(t, []string{"audit", "guard-bash"}, user.Configured.Value.List, "the user directory names both")
	assert.Equal(t, []string{"audit"}, user.Loaded.Value.List, "and supplied one of them")

	proj := hooksField(t, c, diag.KeyHooksProject)
	assert.Equal(t, []string{"guard-bash"}, proj.Configured.Value.List)
	assert.Equal(t, []string{"guard-bash"}, proj.Loaded.Value.List)

	blocking := hooksField(t, c, diag.KeyHooksBlocking)
	assert.Empty(t, blocking.Effective.Value.List,
		"the project's definition replaced the user's whole, fail_closed included")

	assert.Equal(t, []string{"audit"}, hooksField(t, c, diag.KeyHooksAsync).Effective.Value.List)
	assert.Equal(t, []string{"pre_tool", "post_tool"}, hooksField(t, c, diag.KeyHooksEvents).Effective.Value.List)
	assert.Equal(t, []string{"*", "bash"}, hooksField(t, c, diag.KeyHooksTools).Loaded.Value.List)

	timeout := hooksField(t, c, diag.KeyHooksTimeout)
	assert.Equal(t, "5s", timeout.Configured.Value.Str)
	assert.Equal(t, "12s", timeout.Loaded.Value.Str, "the project's budget, not the user's 9s")

	load := hooksField(t, c, diag.KeyHooksLoad)
	assert.Equal(t, string(diag.HookLoadClean), load.Loaded.Value.Str)
	assert.Equal(t, int64(0), load.Effective.Value.Int)
}

// The question must be free to ask. If reading the category ran a hook, a
// developer looking at the harness could deny their own next tool call by
// looking at a list — and would change the answer by reading it.
func TestAskingTheHarnessAboutHooksRunsNoneOfThem(t *testing.T) {
	c, _, marker := developerHooksRuntime(t)

	for range 3 {
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
		require.NoError(t, err)
		_, err = c.diagnostics.Snapshot(t.Context(), "")
		require.NoError(t, err)
		require.NotEmpty(t, c.diagnostics.Catalog().Categories)
		for _, key := range []string{diag.KeyHooksState, diag.KeyHooksRegistered, diag.KeyHooksBlocking} {
			hooksField(t, c, key)
		}
	}

	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "three rounds of questions and no hook script ran")

	// The run path and the marker it would have written are the two things
	// about a hook that must not travel: a path can name a directory the
	// owner chose, and a script can be anything at all.
	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	for _, field := range snapshot.Categories[0].Fields {
		for _, observation := range []diag.Observation{field.Configured, field.Loaded, field.Effective} {
			assert.NotContains(t, observation.Value.Str, marker, field.Key)
			assert.NotContains(t, observation.Value.Str, "run.sh", field.Key)
			assert.NotContains(t, observation.Source.Ref, marker, field.Key)
			for _, item := range observation.Value.List {
				assert.NotContains(t, item, marker, field.Key)
				assert.NotContains(t, item, "run.sh", field.Key)
			}
		}
	}
}

// A directory edited since the load is answered by the load, and a reload is
// what changes the answer. The record has to follow the manager it describes,
// or the view would pair one session's hooks with another's account of them.
func TestTheHarnessAnswersFromTheLoadUntilAReloadReplacesIt(t *testing.T) {
	c, proj, marker := developerHooksRuntime(t)
	before := hooksField(t, c, diag.KeyHooksRegistered)
	require.Equal(t, []string{"audit", "guard-bash"}, before.Effective.Value.List)
	assert.Equal(t, diag.ApplyReload, before.Apply, "which is what the field says it would take")

	writeHooks(t, filepath.Join(proj.HooksDir(), "added"), marker,
		`{"hooks":[{"name":"added","event":"post_turn","run":"./run.sh"}]}`)

	assert.Equal(t, []string{"audit", "guard-bash"},
		hooksField(t, c, diag.KeyHooksRegistered).Effective.Value.List,
		"the view does not read the directory again")

	n, warns, err := c.ReloadHooks()
	require.NoError(t, err)
	require.Empty(t, warns)
	assert.Equal(t, 3, n)

	after := hooksField(t, c, diag.KeyHooksRegistered)
	assert.Equal(t, []string{"added", "audit", "guard-bash"}, after.Effective.Value.List,
		"and the reload is what changed it")
	assert.NotEqual(t, before.Revision, after.Revision, "two snapshots across a reload are of two states")
	assert.Equal(t, []string{"added", "guard-bash"}, hooksField(t, c, diag.KeyHooksProject).Loaded.Value.List,
		"with the new hook's source carried along with it")
}

// Switching hooks off reads no directory at all. The empty answer that follows
// is the consequence, and the lifecycle has to name the cause — otherwise a
// developer goes looking for a hook directory that was never consulted.
func TestTheHarnessSaysWhenTheEnvironmentSwitchedHooksOff(t *testing.T) {
	c, _, _ := developerHooksRuntime(t)
	t.Setenv("COZYPHI_HOOKS", "off")
	_, _, err := c.ReloadHooks()
	require.NoError(t, err)

	state := hooksField(t, c, diag.KeyHooksState)
	assert.Equal(t, string(diag.HooksDisabled), state.Configured.Value.Str)
	assert.Equal(t, "COZYPHI_HOOKS", state.Configured.Source.Ref)
	assert.Equal(t, string(diag.HooksDisabled), state.Effective.Value.Str)

	registered := hooksField(t, c, diag.KeyHooksRegistered)
	assert.Equal(t, diag.StateNotApplicable, registered.Effective.State,
		"an empty list would read as a directory that was read and found nothing")
	assert.Contains(t, registered.Effective.Source.Ref, "switched hooks off")

	assert.Equal(t, string(diag.HookLoadClean), hooksField(t, c, diag.KeyHooksLoad).Loaded.Value.Str,
		"the load still answers, because that is exactly when a reader needs it")
}
