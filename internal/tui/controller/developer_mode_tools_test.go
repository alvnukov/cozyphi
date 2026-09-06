package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/project"
)

// developerToolRuntime starts a granted developer session over an ordinary
// configuration, so the tool layer it reports is the one a user actually sits
// in rather than a fixture's idea of one.
func developerToolRuntime(t *testing.T) *Controller {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	cwd := t.TempDir()

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
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)
	return c
}

func toolField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryTools, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessReportsTheToolsThisSessionActuallyCarries(t *testing.T) {
	c := developerToolRuntime(t)

	registered := toolField(t, c, diag.KeyToolsRegistered)
	assert.Contains(t, registered.Loaded.Value.List, "read")
	assert.Contains(t, registered.Loaded.Value.List, "harness",
		"the session was granted developer mode, so it carries the tool asking the question")

	assert.Equal(t, diag.ToolRegistered, toolField(t, c, diag.ToolKey("read")).Loaded.Value.Str)
	assert.Equal(t, diag.ToolRestricted, toolField(t, c, diag.ToolKey("read")).Effective.Value.Str,
		"in useplan no approved step is in progress, so the plan gate is the nearer answer")

	// Out of the plan gate's way, the remaining condition is the call's own
	// arguments — which is as far as this view is allowed to go.
	c.SetMode(agent.ModeBuild)
	assert.Equal(t, diag.ToolRequiresArgumentCheck,
		toolField(t, c, diag.ToolKey("read")).Effective.Value.Str,
		"a path decides a read, and no path has been written yet")
	assert.Equal(t, diag.ToolRegistered, toolField(t, c, diag.ToolKey("harness")).Effective.Value.Str,
		"a tool the boundary judges by name alone carries nothing left to check")
}

func TestAToolWithNoOwnerIsExplainedRatherThanOmitted(t *testing.T) {
	c := developerToolRuntime(t)

	// A temp workspace has no task ledger, so the tool that works one is not
	// registered — and that is a fact about this workspace, not a denial.
	field := toolField(t, c, diag.ToolKey("task"))

	assert.Equal(t, diag.ToolUnavailable, field.Loaded.Value.Str)
	assert.Equal(t, diag.ToolUnavailable, field.Effective.Value.Str)
	assert.Contains(t, field.Effective.Source.Ref, "task registry",
		"the answer says what would have to exist for the tool to be there")
}

func TestThePostureIsObservedWhereTheSessionStandsInIt(t *testing.T) {
	c := developerToolRuntime(t)
	assert.Equal(t, "useplan", toolField(t, c, diag.KeyToolsMode).Effective.Value.Str)
	assert.Equal(t, "deny", toolField(t, c, diag.KeyToolsPlanGate).Effective.Value.Str)

	c.SetMode(agent.ModePlan)

	assert.Equal(t, "plan", toolField(t, c, diag.KeyToolsMode).Effective.Value.Str)
	write := toolField(t, c, diag.ToolKey("write"))
	assert.Equal(t, diag.ToolRegistered, write.Configured.Value.Str,
		"the session still carries the tool")
	assert.Equal(t, diag.ToolUnavailable, write.Loaded.Value.Str,
		"the posture narrowed the registry the model is offered")
}

func TestTheToolAnswerFollowsTheEngineAcrossARebind(t *testing.T) {
	c := developerToolRuntime(t)
	before := toolField(t, c, diag.KeyToolsRegistered).Revision
	require.NotEmpty(t, before)

	c.SetMode(agent.ModePlan)

	assert.NotEqual(t, before, toolField(t, c, diag.KeyToolsRegistered).Revision,
		"a rebind produces a different list, and the revision says so")
}

func TestObservingToolsAsksTheBoundaryNothingAndMovesNoPlanStep(t *testing.T) {
	c := developerToolRuntime(t)
	before := c.engineRef.Load().ToolNames()
	plan := c.engineRef.Load().Plan()

	for range 3 {
		toolField(t, c, diag.KeyToolsRegistered)
	}

	assert.Equal(t, before, c.engineRef.Load().ToolNames(),
		"an observation neither registers nor removes a tool")
	assert.Equal(t, plan, c.engineRef.Load().Plan(),
		"and it leaves the plan exactly where it stood")
}

func TestNoEngineYetLeavesTheToolCategoryUnavailable(t *testing.T) {
	c := &Controller{}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(),
		diag.NewToolCollector(diag.ToolDeps{State: c.toolState}))

	explained, err := registry.Explain(t.Context(), diag.CategoryTools, diag.KeyToolsRegistered)
	require.NoError(t, err)
	assert.Equal(t, diag.StateUnavailable, explained.Field.Effective.State)
}
