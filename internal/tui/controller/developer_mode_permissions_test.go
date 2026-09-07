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
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
)

// The rules below are what a user writes into a permissions block. Each one
// can name a host, an internal command or a private server, so none of them
// may come back out of the harness view.
const (
	bashAllowSentinel = "^deploy --to prod-cluster-sentinel"
	bashDenySentinel  = "internal-vault-sentinel"
	mcpAllowSentinel  = "^vault-server-sentinel/"
)

// permissionsBlock is a permissions section whose every rule the file sets to
// something the built-in policy does not hold, so every observed value belongs
// to the configuration rather than to the defaults.
const permissionsBlock = `permissions:
  mode: autopilot
  ask_timeout_sec: 45
  tasks: read
  bash:
    default: ask
    allow:
      - "` + bashAllowSentinel + `"
    deny:
      - "` + bashDenySentinel + `"
  mcp:
    allow:
      - "` + mcpAllowSentinel + `"
`

// developerPermissionRuntime starts a granted developer session over a config
// file with that permissions block, so the configured layer has a real file to
// name and the assembled boundary has real rules to hold.
func developerPermissionRuntime(t *testing.T) *Controller {
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
	require.NoError(t, os.WriteFile(configFile,
		[]byte(twoModelConfig("alpha")+permissionsBlock), 0o600))

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

// permissionField observes one field of the permission category through the
// session's own registry, the way the harness tool does.
func permissionField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryPermissions, key)
	require.NoError(t, err)
	return explained.Field
}

func TestTheHarnessObservesTheBoundaryTheSessionJudgesWith(t *testing.T) {
	c := developerPermissionRuntime(t)

	gate := permissionField(t, c, diag.KeyPermissionGate)
	assert.Equal(t, "static", gate.Effective.Value.Str, "an ordinary session runs on a compiled ruleset")

	mode := permissionField(t, c, diag.KeyPermissionMode)
	assert.Equal(t, "autopilot", mode.Configured.Value.Str)
	assert.Equal(t, "autopilot", mode.Effective.Value.Str)
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "permissions.mode"}, mode.Effective.Source)

	writes := permissionField(t, c, diag.KeyPermissionWrites)
	assert.True(t, writes.Effective.Value.Bool)
	assert.Equal(t, diag.SourceDefault, writes.Effective.Source.Kind,
		"a rule the file left alone came from the built-in policy")
	assert.Contains(t, writes.Effective.Source.Ref, "permissions.workspace_only_writes",
		"and the answer still says which key would replace it")

	allow := permissionField(t, c, diag.KeyPermissionBashAllow)
	assert.Equal(t, int64(1), allow.Effective.Value.Int, "the file replaced the built-in list whole")
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "permissions.bash.allow"}, allow.Effective.Source)
	assert.Equal(t, int64(1), permissionField(t, c, diag.KeyPermissionBashDeny).Effective.Value.Int)
	assert.Equal(t, int64(1), permissionField(t, c, diag.KeyPermissionMCPAllow).Effective.Value.Int)
	assert.Equal(t, int64(45), permissionField(t, c, diag.KeyPermissionAskTimeout).Effective.Value.Int)
	assert.Equal(t, "read", permissionField(t, c, diag.KeyPermissionTasks).Effective.Value.Str)

	sensitive := permissionField(t, c, diag.KeyPermissionSensitive)
	assert.Positive(t, sensitive.Effective.Value.Int, "the built-in refusals are in force too")
	assert.Equal(t, diag.SourceDefault, sensitive.Effective.Source.Kind)
	assert.Contains(t, sensitive.Effective.Source.Ref, "no configuration key sets it")

	memory := permissionField(t, c, diag.KeyPermissionMemory)
	assert.True(t, memory.Effective.Value.Bool,
		"the session binds its own memory directory while assembling the gate")
	assert.Equal(t, diag.StateNotApplicable, memory.Configured.State)

	bypass := permissionField(t, c, diag.KeyPermissionBypass)
	assert.Equal(t, diag.StateUnset, bypass.Configured.State, "the file did not ask for allow-all")
	assert.True(t, bypass.Loaded.Value.Bool, "the session's switch stands in front of the boundary")
	assert.False(t, bypass.Effective.Value.Bool, "and it is off")
}

func TestPlanModeIsObservedAsTheOverlayItIs(t *testing.T) {
	c := developerPermissionRuntime(t)
	c.SetMode(agent.ModePlan)

	mode := permissionField(t, c, diag.KeyPermissionMode)
	assert.Equal(t, "autopilot", mode.Configured.Value.Str, "the file is unchanged by a mode switch")
	assert.Equal(t, "readonly", mode.Loaded.Value.Str, "the assembled boundary is what decides")
	assert.Equal(t, diag.SourcePlan, mode.Loaded.Source.Kind)
	assert.Equal(t, "readonly", mode.Effective.Value.Str)

	// Leaving plan mode rebuilds the gate from the configuration, and the
	// observation follows the boundary rather than remembering the overlay.
	c.SetMode(agent.ModeBuild)
	assert.Equal(t, "autopilot", permissionField(t, c, diag.KeyPermissionMode).Effective.Value.Str)
	assert.Equal(t, diag.SourceConfigFile,
		permissionField(t, c, diag.KeyPermissionMode).Loaded.Source.Kind)
}

func TestTheAllowAllSwitchSuspendsTheRulesWithoutErasingThem(t *testing.T) {
	c := developerPermissionRuntime(t)
	c.SetAllowAll(true)

	deny := permissionField(t, c, diag.KeyPermissionBashDeny)
	assert.Equal(t, int64(1), deny.Loaded.Value.Int, "the rule is still loaded")
	assert.Equal(t, diag.StateNotApplicable, deny.Effective.State, "and it is not deciding anything")
	assert.Equal(t, diag.SourceSession, deny.Effective.Source.Kind)
	assert.Contains(t, deny.Effective.Source.Ref, "allow-all switch")

	bypass := permissionField(t, c, diag.KeyPermissionBypass)
	assert.True(t, bypass.Effective.Value.Bool)
	assert.Equal(t, "static", permissionField(t, c, diag.KeyPermissionGate).Effective.Value.Str,
		"the boundary behind the switch has not been replaced")

	c.SetAllowAll(false)
	restored := permissionField(t, c, diag.KeyPermissionBashDeny)
	assert.Equal(t, diag.StatePresent, restored.Effective.State, "turning the switch off puts the rules back")
	assert.Equal(t, int64(1), restored.Effective.Value.Int)
}

func TestThePermissionViewCarriesNoRuleTheUserWrote(t *testing.T) {
	c := developerPermissionRuntime(t)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPermissions)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)

	rendered := snapshot.JSON()
	for _, secret := range []string{
		bashAllowSentinel, bashDenySentinel, mcpAllowSentinel, "sentinel",
		"config-file-secret", c.memory.Dir(), c.cwd,
	} {
		assert.NotContains(t, rendered, secret, "a rule is counted and attributed, never quoted")
	}
	assert.Contains(t, rendered, "static", "what is deciding is still nameable")
	assert.Contains(t, rendered, "autopilot")
}

func TestObservingPermissionsDecidesNothingAndGrantsNothing(t *testing.T) {
	c := developerPermissionRuntime(t)
	gate := c.currentGate()
	requests := []permission.Request{
		{Action: permission.ActionBash, Command: "deploy --to prod-cluster-sentinel"},
		{Action: permission.ActionBash, Command: "curl internal-vault-sentinel"},
		{Action: permission.ActionBash, Command: "ls -la"},
		{Action: permission.ActionWrite, Paths: []string{filepath.Join(c.cwd, "main.go")}},
		{Action: permission.ActionWrite, Paths: []string{filepath.Join(t.TempDir(), "outside.go")}},
		{Action: permission.ActionMCPCall, Target: "vault-server-sentinel/read"},
		{Action: permission.ActionTaskWrite, Target: "some-task"},
	}

	before := gateDecisions(t, gate, requests)
	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPermissions)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	after := gateDecisions(t, gate, requests)

	assert.Equal(t, before, after, "an observation reads the boundary; it never probes it")
	assert.Same(t, gate, c.currentGate(), "and it never rebuilds one either")
}

func gateDecisions(t *testing.T, gate permission.Gate, requests []permission.Request) []string {
	t.Helper()
	out := make([]string, 0, len(requests))
	for _, req := range requests {
		decision, reason := gate.Check(t.Context(), req)
		out = append(out, decision.String()+": "+reason)
	}
	return out
}

func TestTheTUIPublishesTheWholePermissionCategory(t *testing.T) {
	c := developerPermissionRuntime(t)

	var entry diag.CatalogEntry
	for _, candidate := range c.diagnostics.Catalog().Categories {
		if candidate.Category == diag.CategoryPermissions {
			entry = candidate
		}
	}

	require.Equal(t, diag.CategoryPermissions, entry.Category, "the TUI registers the collector")
	assert.Equal(t, diag.AvailabilityAvailable, entry.Availability)
	assert.Equal(t, []string{
		"gate", "mode", "bypass", "bash.default", "bash.allow", "bash.deny",
		"workspace.only_writes", "workspace.only_reads", "paths.sensitive", "mcp.allow",
		"web.allow", "tasks", "memory", "ask_timeout_sec", "source_order",
	}, entry.Keys, "the same key set the headless entry point declares")
	assert.Contains(t, entry.Reason, "never quoted")
}

func TestTheOverlayNamesWhatNarrowedTheConfiguredRules(t *testing.T) {
	var missing *Controller
	assert.Equal(t, diag.Source{}, missing.permissionOverlay(), "a session that does not exist narrows nothing")
	assert.Equal(t, diag.Source{}, (&Controller{mode: agent.ModeBuild}).permissionOverlay(),
		"an ordinary session runs the configured rules, and a difference is not this one's to explain")

	plan := (&Controller{mode: agent.ModePlan}).permissionOverlay()
	assert.Equal(t, diag.SourcePlan, plan.Kind)
	assert.Contains(t, plan.Ref, "readonly")

	// A sub-agent runs under its role's ceiling whatever its mode is, so the
	// ceiling is what is named — it is the stronger of the two claims.
	child := (&Controller{childRole: job.RoleWorker, mode: agent.ModePlan}).permissionOverlay()
	assert.Equal(t, diag.SourceComputed, child.Kind)
	assert.Contains(t, child.Ref, "ceiling")
	assert.Contains(t, child.Ref, "never widens")
}
