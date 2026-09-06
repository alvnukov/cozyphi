package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
)

// developerProject stands up an isolated project whose model resolves without
// a network: every assertion here is about which tools a session was built
// with, never about what a model answered.
func developerProject(t *testing.T) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")
	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	return proj, cwd
}

// developerRuntime returns a runtime and its workspace, granted or not.
func developerRuntime(t *testing.T, granted bool) (*Runtime, *Workspace) {
	t.Helper()
	proj, cwd := developerProject(t)
	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	if granted {
		require.NoError(t, rt.GrantDeveloperMode())
	}
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	return rt, ws
}

func TestUserSessionsObserveTheHarnessOnlyWhenGranted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		granted bool
	}{
		{"granted", true},
		{"not granted", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, ws := developerRuntime(t, tc.granted)
			c, err := rt.NewSession(NewBus(nil), ws, "", nil)
			require.NoError(t, err)

			assert.Equal(t, tc.granted, c.engine.HasTool("harness"))
			assert.True(t, c.engine.HasTool("plan"), "developer mode changes no other capability")
			assert.True(t, c.engine.HasTool("read"))
		})
	}
}

func TestEverySessionOfADeveloperProcessObservesItsOwnIdentity(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	first, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	second, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.NotEqual(t, first.SessionID(), second.SessionID())

	for _, c := range []*Controller{first, second} {
		require.True(t, c.engine.HasTool("harness"))
		explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
		require.NoError(t, err)
		assert.Equal(t, c.SessionID(), explained.Field.Effective.Value.Str,
			"a session must observe itself, not whichever session is active")
	}

	snapshot, err := first.diagnostics.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	var mode string
	for _, field := range snapshot.Categories[0].Fields {
		if field.Key == diag.KeyMode {
			mode = field.Effective.Value.Str
		}
	}
	assert.Equal(t, "tui", mode, "the same contract, reported from the entry point it runs in")
}

func TestChildSessionsNeverInheritTheCapability(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	rt.EnableInteractiveChildren()
	parent, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.True(t, parent.engine.HasTool("harness"))

	child, err := rt.newChild(parent, job.Meta{
		ID: "job-1", Role: job.RoleExplore, ParentID: "parent-conversation", WorkDir: ws.cwd,
	}, &agent.EngineOpts{
		Model:       parent.ModelConfig(),
		MaxRounds:   4,
		SessionOpts: agent.SessionOpts{Cwd: ws.cwd, SessionDir: parent.SessionDir(), Persist: true},
	})
	require.NoError(t, err)
	t.Cleanup(child.Close)

	assert.Nil(t, child.diagnostics)
	assert.False(t, child.engine.HasTool("harness"),
		"the capability is the user's; a sub-agent of a developer session is still a sub-agent")
}

func TestGrantIsRefusedOnceASessionExists(t *testing.T) {
	rt, ws := developerRuntime(t, false)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	require.Error(t, rt.GrantDeveloperMode(), "the capability is fixed at startup")
	assert.False(t, c.engine.HasTool("harness"))

	next, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	assert.False(t, next.engine.HasTool("harness"), "a refused grant grants nothing later either")
}

func TestResumingADeveloperSessionElsewhereRestoresNoCapability(t *testing.T) {
	granted, ws := developerRuntime(t, true)
	first, err := granted.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	require.True(t, first.engine.HasTool("harness"))
	// A session with no turns has nothing on disk to resume.
	require.NoError(t, first.engine.Session().Append(llm.Message{
		Role: llm.RoleAssistant, Content: "observed in a developer process",
	}))
	resumePath := first.SessionFile()
	require.NotEmpty(t, resumePath)
	first.Close()
	require.NoError(t, granted.Close())

	proj, err := project.Discover(ws.cwd)
	require.NoError(t, err)
	plain, err := NewRuntime(proj)
	require.NoError(t, err)
	defer func() { _ = plain.Close() }()
	plainWS, err := plain.Workspace(ws.cwd)
	require.NoError(t, err)

	resumed, err := plain.NewSession(NewBus(nil), plainWS, resumePath, nil)
	require.NoError(t, err)
	assert.False(t, resumed.engine.HasTool("harness"),
		"a session's history is a transcript, not a stored permission")
}

func TestConfigAndEnvironmentGrantNothing(t *testing.T) {
	proj, cwd := developerProject(t)
	for _, name := range []string{"COZYPHI_DEVELOPER_MODE", "COZYPHI_DEVELOPER", "DEVELOPER_MODE"} {
		t.Setenv(name, "1")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(proj.Global().ConfigFile()), 0o755))
	require.NoError(t, os.WriteFile(proj.Global().ConfigFile(),
		[]byte("developerMode: true\ndeveloper_mode: true\ndeveloper: true\n"), 0o600))

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	defer func() { _ = rt.Close() }()
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	assert.False(t, c.engine.HasTool("harness"), "only the command line grants the capability")
}

func TestClearKeepsTheCapabilityAndFollowsTheNewSession(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	before := c.SessionID()

	require.NoError(t, c.Clear())
	require.NotEqual(t, before, c.SessionID(), "clear replaces the engine")

	assert.True(t, c.engine.HasTool("harness"), "the capability belongs to the process, not the engine")
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
	require.NoError(t, err)
	assert.Equal(t, c.SessionID(), explained.Field.Effective.Value.Str,
		"the observation follows the replacement, it does not report the closed session")
}

func TestClosingOneDeveloperSessionLeavesTheOthersObserving(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	first, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	second, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	secondID := second.SessionID()

	first.Close()

	require.True(t, second.engine.HasTool("harness"), "a closed session takes no capability with it")
	explained, err := second.diagnostics.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
	require.NoError(t, err)
	assert.Equal(t, secondID, explained.Field.Effective.Value.Str)

	third, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	assert.True(t, third.engine.HasTool("harness"), "the process still holds the capability")
}
