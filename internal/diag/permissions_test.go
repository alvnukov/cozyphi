package diag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// configuredPolicy is a permissions block as the loader resolves it: the
// built-in lists, interactive mode, containment on.
func configuredPolicy() diag.PermissionFacts {
	return diag.PermissionFacts{
		Known:               true,
		Mode:                "interactive",
		BashDefault:         "ask",
		BashAllow:           12,
		BashDeny:            7,
		BashAllowIsDefault:  true,
		BashDenyIsDefault:   true,
		SensitivePaths:      5,
		MCPAllow:            0,
		WorkspaceOnlyWrites: true,
		WorkspaceOnlyReads:  true,
		AskTimeoutSec:       120,
		Tasks:               "write",
	}
}

// staticGate is that policy assembled into a boundary with nothing in front
// of it — the ordinary session.
func staticGate(policy diag.PermissionFacts) diag.GateFacts {
	policy.Memory = true
	return diag.GateFacts{Known: true, Kind: diag.GateStatic, Policy: policy}
}

func permissionFields(t *testing.T, deps diag.PermissionDeps) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewPermissionCollector(deps))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryPermissions)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	return snapshot.Categories[0].Fields
}

// permissionDeps wires the two owners and the overlay the entry point names.
// The defaults are left unwired here, so a value is attributed to the key that
// sets it; the test below is the one about telling a default apart.
func permissionDeps(
	configured diag.PermissionFacts, gate diag.GateFacts, overlay diag.Source,
) diag.PermissionDeps {
	return diag.PermissionDeps{
		Configured: func() diag.PermissionFacts { return configured },
		Gate:       func() diag.GateFacts { return gate },
		Overlay:    func() diag.Source { return overlay },
	}
}

func TestAnOrdinarySessionReportsTheRulesItIsActuallyRunning(t *testing.T) {
	configured := configuredPolicy()
	fields := permissionFields(t, permissionDeps(configured, staticGate(configured), diag.Source{}))

	gate := fieldByKey(t, fields, diag.KeyPermissionGate)
	assert.Equal(t, diag.StateNotApplicable, gate.Configured.State, "nothing configures a gate's shape")
	assert.Equal(t, "static", gate.Effective.Value.Str)
	assert.Equal(t, diag.SourceComputed, gate.Effective.Source.Kind)

	mode := fieldByKey(t, fields, diag.KeyPermissionMode)
	assert.Equal(t, "interactive", mode.Configured.Value.Str)
	assert.Equal(t, "interactive", mode.Effective.Value.Str)
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "permissions.mode"}, mode.Loaded.Source,
		"a loaded value the configuration asked for keeps the configuration's origin")
	assert.Equal(t, diag.ApplyReload, mode.Apply, "a mode is folded when the boundary is built")

	allow := fieldByKey(t, fields, diag.KeyPermissionBashAllow)
	assert.Equal(t, int64(12), allow.Effective.Value.Int)
	assert.Equal(t, diag.SourceDefault, allow.Loaded.Source.Kind, "an untouched list is the built-in one")
	assert.Contains(t, allow.Loaded.Source.Ref, "permissions.bash.allow",
		"and the answer says which key would replace it")

	bypass := fieldByKey(t, fields, diag.KeyPermissionBypass)
	assert.Equal(t, diag.StateUnset, bypass.Configured.State, "the file did not ask for allow-all")
	assert.False(t, bypass.Loaded.Value.Bool, "nothing in front of the boundary can hold it open")
	assert.False(t, bypass.Effective.Value.Bool)

	memory := fieldByKey(t, fields, diag.KeyPermissionMemory)
	assert.Equal(t, diag.StateNotApplicable, memory.Configured.State, "no config key binds a memory directory")
	assert.True(t, memory.Effective.Value.Bool)
	assert.Equal(t, diag.ApplyNewSession, memory.Apply)

	assert.Equal(t, []string{"default", "config_file", "cli_flag", "plan", "session"},
		fieldByKey(t, fields, diag.KeyPermissionSourceOrder).Effective.Value.List)
}

func TestAnOverlayThatNarrowedTheRulesIsNamedWhereTheyDiffer(t *testing.T) {
	configured := configuredPolicy()
	narrowed := configured
	narrowed.Mode = "readonly"
	narrowed.Tasks = "read"
	overlay := diag.Source{Kind: diag.SourcePlan, Ref: "plan mode overlays readonly"}

	fields := permissionFields(t, permissionDeps(configured, staticGate(narrowed), overlay))

	mode := fieldByKey(t, fields, diag.KeyPermissionMode)
	assert.Equal(t, "interactive", mode.Configured.Value.Str, "the file still says what it says")
	assert.Equal(t, "readonly", mode.Loaded.Value.Str, "the boundary is what decides")
	assert.Equal(t, overlay, mode.Loaded.Source, "and the difference is attributed, not left dangling")
	assert.Equal(t, "readonly", mode.Effective.Value.Str)

	tasksField := fieldByKey(t, fields, diag.KeyPermissionTasks)
	assert.Equal(t, "read", tasksField.Effective.Value.Str)
	assert.Equal(t, overlay, tasksField.Effective.Source)
	assert.Equal(t, diag.ApplyImmediate, tasksField.Apply, "the task level is consulted per call")

	writes := fieldByKey(t, fields, diag.KeyPermissionWrites)
	assert.Equal(t, diag.SourceConfigFile, writes.Loaded.Source.Kind,
		"a rule the overlay left alone keeps the origin that set it")
}

// An egress destination is a rule like the others and lives with them: it is
// compiled into the same boundary and matched against the host a web call
// would reach, so a reader looking for what this session may talk to finds it
// beside bash.allow and mcp.allow rather than in a category about a tool.
func TestAnEgressAllowEntryIsCountedBesideTheOtherRules(t *testing.T) {
	configured := configuredPolicy()
	configured.WebAllow = 3
	fields := permissionFields(t, permissionDeps(configured, staticGate(configured), diag.Source{}))

	web := fieldByKey(t, fields, diag.KeyPermissionWebAllow)
	assert.Equal(t, int64(3), web.Configured.Value.Int)
	assert.Equal(t, int64(3), web.Effective.Value.Int)
	assert.Equal(t, diag.ApplyReload, web.Apply, "the boundary is compiled again when the file is read again")
	assert.Contains(t, web.Configured.Source.Ref, "never quoted")
	assert.Empty(t, web.Configured.Value.List, "there is nowhere for a host to be written")
	assert.Empty(t, web.Configured.Value.Str)
}

func TestADifferenceNobodyClaimedIsDatedRatherThanInvented(t *testing.T) {
	configured := configuredPolicy()
	changed := configured
	changed.Tasks = "read"

	fields := permissionFields(t, permissionDeps(configured, staticGate(changed), diag.Source{}))

	tasksField := fieldByKey(t, fields, diag.KeyPermissionTasks)
	assert.Equal(t, diag.SourceSession, tasksField.Loaded.Source.Kind)
	assert.Contains(t, tasksField.Loaded.Source.Ref, "after the configuration was loaded")
}

func TestABypassedBoundaryShowsTheRulesAndSaysNoneIsApplying(t *testing.T) {
	configured := configuredPolicy()
	gate := staticGate(configured)
	gate.Bypassable = true
	gate.Bypassing = true

	fields := permissionFields(t, permissionDeps(configured, gate, diag.Source{}))

	for _, key := range []string{
		diag.KeyPermissionMode, diag.KeyPermissionBashDefault, diag.KeyPermissionBashAllow,
		diag.KeyPermissionBashDeny, diag.KeyPermissionWrites, diag.KeyPermissionReads,
		diag.KeyPermissionSensitive, diag.KeyPermissionMCPAllow, diag.KeyPermissionWebAllow,
		diag.KeyPermissionTasks,
		diag.KeyPermissionMemory, diag.KeyPermissionAskTimeout,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State,
			"%s is loaded but is not deciding anything while the boundary is open", key)
		assert.Equal(t, diag.SourceSession, field.Effective.Source.Kind, "%s names the switch holding it open", key)
	}

	allow := fieldByKey(t, fields, diag.KeyPermissionBashAllow)
	assert.Equal(t, diag.StatePresent, allow.Loaded.State, "the rules are still there to be turned back on")
	assert.Equal(t, int64(12), allow.Loaded.Value.Int)

	bypass := fieldByKey(t, fields, diag.KeyPermissionBypass)
	assert.True(t, bypass.Loaded.Value.Bool)
	assert.True(t, bypass.Effective.Value.Bool)
	assert.Equal(t, diag.ApplyImmediate, bypass.Apply, "the switch takes effect on the next request")
}

func TestABoundaryWithNoRulesIsNotReportedAsRulesThatPermit(t *testing.T) {
	configured := configuredPolicy()
	configured.AllowAll = true
	gate := diag.GateFacts{
		Known:      true,
		Kind:       diag.GateAllowAll,
		Reason:     "no rules at all",
		Bypassable: true,
		Bypassing:  true,
	}

	fields := permissionFields(t, permissionDeps(configured, gate, diag.Source{}))

	assert.Equal(t, "allow_all", fieldByKey(t, fields, diag.KeyPermissionGate).Effective.Value.Str)

	mode := fieldByKey(t, fields, diag.KeyPermissionMode)
	assert.Equal(t, "interactive", mode.Configured.Value.Str, "the file's answer is still reportable")
	assert.Equal(t, diag.StateUnavailable, mode.Loaded.State,
		"there is no boundary holding a mode, so none is reported")
	assert.Equal(t, diag.StateUnavailable, mode.Effective.State)

	bypass := fieldByKey(t, fields, diag.KeyPermissionBypass)
	assert.True(t, bypass.Configured.Value.Bool)
	assert.True(t, bypass.Effective.Value.Bool)
	assert.Equal(t, diag.SourceCLIFlag, bypass.Effective.Source.Kind,
		"a gate assembled without rules was made that way before the session started")
}

func TestABoundaryNobodyRecognizesReportsNoRulesRatherThanTheConfiguredOnes(t *testing.T) {
	configured := configuredPolicy()
	gate := diag.GateFacts{
		Known:  true,
		Kind:   diag.GateUnknown,
		Reason: "the installed boundary is not one this view can read",
	}

	fields := permissionFields(t, permissionDeps(configured, gate, diag.Source{}))

	gateField := fieldByKey(t, fields, diag.KeyPermissionGate)
	assert.Equal(t, "unknown", gateField.Effective.Value.Str)
	assert.Equal(t, gate.Reason, gateField.Effective.Source.Ref, "an unreadable boundary says why")

	for _, key := range []string{
		diag.KeyPermissionMode, diag.KeyPermissionBashDefault, diag.KeyPermissionBashAllow,
		diag.KeyPermissionWrites, diag.KeyPermissionSensitive, diag.KeyPermissionTasks,
		diag.KeyPermissionMemory, diag.KeyPermissionAskTimeout,
	} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State,
			"%s cannot be read off a boundary this view does not understand", key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State,
			"%s is not filled in from the configuration, which is the substitution to avoid", key)
	}

	assert.Equal(t, "interactive", fieldByKey(t, fields, diag.KeyPermissionMode).Configured.Value.Str,
		"the configured layer is still answerable; it is just not the answer")

	bypass := fieldByKey(t, fields, diag.KeyPermissionBypass)
	assert.False(t, bypass.Effective.Value.Bool, "unknown is not allow-all")
}

func TestAFailedAssemblyCarriesItsReasonAndDeniesNothingIntoTheOpen(t *testing.T) {
	fields := permissionFields(t, permissionDeps(configuredPolicy(),
		diag.GateFacts{Known: true, Kind: diag.GateUnavailable, Reason: "bash deny: invalid pattern"},
		diag.Source{}))

	gate := fieldByKey(t, fields, diag.KeyPermissionGate)
	assert.Equal(t, "unavailable", gate.Effective.Value.Str)
	assert.Equal(t, "bash deny: invalid pattern", gate.Effective.Source.Ref)
	assert.Equal(t, diag.StateUnavailable, fieldByKey(t, fields, diag.KeyPermissionBashDeny).Effective.State)
	assert.False(t, fieldByKey(t, fields, diag.KeyPermissionBypass).Effective.Value.Bool)
}

func TestWithNoOwnersWiredEveryPermissionLayerSaysSo(t *testing.T) {
	fields := permissionFields(t, diag.PermissionDeps{})

	require.Len(t, fields, 15, "the declared key set is answered whether or not it can be observed")
	gate := fieldByKey(t, fields, diag.KeyPermissionGate)
	assert.Equal(t, diag.StateUnavailable, gate.Effective.State, "no boundary was published")

	for _, key := range []string{diag.KeyPermissionMode, diag.KeyPermissionBashAllow, diag.KeyPermissionTasks} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Configured.State, "%s has no configuration to read", key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State)
	}

	assert.Equal(t, diag.StatePresent, fieldByKey(t, fields, diag.KeyPermissionSourceOrder).Effective.State,
		"the precedence is compiled in, so it is answerable with nothing wired")
}

func TestTheCatalogDeclaresEveryPermissionFieldWithoutObservingOne(t *testing.T) {
	observed := 0
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewPermissionCollector(diag.PermissionDeps{
			Gate: func() diag.GateFacts {
				observed++
				return diag.GateFacts{}
			},
		}))

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryPermissions {
			entry = candidate
		}
	}

	require.Equal(t, diag.CategoryPermissions, entry.Category)
	assert.Equal(t, diag.AvailabilityAvailable, entry.Availability)
	assert.Equal(t, []string{
		"gate", "mode", "bypass", "bash.default", "bash.allow", "bash.deny",
		"workspace.only_writes", "workspace.only_reads", "paths.sensitive", "mcp.allow",
		"web.allow", "tasks", "memory", "ask_timeout_sec", "source_order",
	}, entry.Keys)
	assert.Contains(t, entry.Reason, "never quoted")
	assert.Zero(t, observed, "listing what can be asked reads no boundary")
}

func TestAskingForOnePermissionFieldAnswersThatFieldAlone(t *testing.T) {
	configured := configuredPolicy()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewPermissionCollector(permissionDeps(configured, staticGate(configured), diag.Source{})))

	explained, err := registry.Explain(t.Context(), diag.CategoryPermissions, diag.KeyPermissionSensitive)
	require.NoError(t, err)
	assert.Equal(t, diag.KeyPermissionSensitive, explained.Field.Key)
	assert.Equal(t, int64(5), explained.Field.Effective.Value.Int, "a count is the whole of what a path list says")
	assert.Equal(t, diag.SourceDefault, explained.Field.Effective.Source.Kind)
	assert.Contains(t, explained.Field.Effective.Source.Ref, "no configuration key sets it")

	_, err = registry.Explain(t.Context(), diag.CategoryPermissions, "bash.allow.patterns")
	require.Error(t, err, "a key that would quote a rule is not a key this category has")
}

func TestARuleNobodyTouchedIsAttributedToTheBuiltInPolicy(t *testing.T) {
	defaults := configuredPolicy()
	configured := defaults
	configured.AskTimeoutSec = 45
	configured.Tasks = "read"

	deps := permissionDeps(configured, staticGate(configured), diag.Source{})
	deps.Defaults = func() diag.PermissionFacts { return defaults }
	fields := permissionFields(t, deps)

	mode := fieldByKey(t, fields, diag.KeyPermissionMode)
	assert.Equal(t, diag.SourceDefault, mode.Configured.Source.Kind,
		"a value the file left alone came from the built-in policy, not from the file")
	assert.Contains(t, mode.Configured.Source.Ref, "permissions.mode",
		"and the answer still says which key would replace it")
	assert.Equal(t, diag.SourceDefault, mode.Effective.Source.Kind)

	timeout := fieldByKey(t, fields, diag.KeyPermissionAskTimeout)
	assert.Equal(t, diag.Source{Kind: diag.SourceConfigFile, Ref: "permissions.ask_timeout_sec"},
		timeout.Configured.Source, "a value that differs from the default is the file's")
	assert.Equal(t, int64(45), timeout.Effective.Value.Int)
	assert.Equal(t, diag.SourceConfigFile,
		fieldByKey(t, fields, diag.KeyPermissionTasks).Effective.Source.Kind)
}

func TestABoundaryNarrowedBackOntoTheDefaultIsStillTheOverlaysDoing(t *testing.T) {
	defaults := configuredPolicy()
	configured := defaults
	configured.Mode = "autopilot"
	overlay := diag.Source{Kind: diag.SourcePlan, Ref: "plan mode overlays readonly"}

	deps := permissionDeps(configured, staticGate(defaults), overlay)
	deps.Defaults = func() diag.PermissionFacts { return defaults }
	fields := permissionFields(t, deps)

	mode := fieldByKey(t, fields, diag.KeyPermissionMode)
	assert.Equal(t, diag.SourceConfigFile, mode.Configured.Source.Kind, "the file asked for autopilot")
	assert.Equal(t, "interactive", mode.Loaded.Value.Str)
	assert.Equal(t, overlay, mode.Loaded.Source,
		"a loaded value that differs is the overlay's, whatever it happens to coincide with")
}
