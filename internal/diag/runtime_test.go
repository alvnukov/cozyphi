package diag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

func runtimeRegistry(t *testing.T, deps diag.RuntimeDeps) *diag.Registry {
	t.Helper()
	return diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewRuntimeCollector(deps))
}

func fieldByKey(t *testing.T, fields []diag.Field, key string) diag.Field {
	t.Helper()
	for _, field := range fields {
		if field.Key == key {
			return field
		}
	}
	t.Fatalf("no field %q in %d fields", key, len(fields))
	return diag.Field{}
}

func TestRuntimeCollectorAnswersTheProcessItIsIn(t *testing.T) {
	registry := runtimeRegistry(t, diag.RuntimeDeps{
		Version:   "1.4.2",
		Mode:      "headless",
		Enabled:   true,
		Workspace: func() string { return "/srv/work" },
		SessionID: func() string { return "sess-1" },
	})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	fields := snapshot.Categories[0].Fields

	version := fieldByKey(t, fields, diag.KeyVersion)
	assert.Equal(t, "1.4.2", version.Effective.Value.Str)
	assert.Equal(t, diag.SourceBuild, version.Effective.Source.Kind)

	mode := fieldByKey(t, fields, diag.KeyMode)
	assert.Equal(t, "headless", mode.Effective.Value.Str)
	assert.Equal(t, diag.SourceComputed, mode.Effective.Source.Kind)

	developer := fieldByKey(t, fields, diag.KeyDeveloperMode)
	assert.True(t, developer.Effective.Value.Bool)
	assert.Equal(t, diag.SourceCLIFlag, developer.Configured.Source.Kind)
	assert.Equal(t, "--developer-mode", developer.Configured.Source.Ref)
	assert.Equal(t, diag.ApplyRestart, developer.Apply, "the flag is only readable at startup")
	assert.Equal(t, diag.ScopeProcess, developer.Scope)

	assert.Equal(t, "/srv/work", fieldByKey(t, fields, diag.KeyWorkspaceRoot).Effective.Value.Str)
	assert.Equal(t, "sess-1", fieldByKey(t, fields, diag.KeySessionID).Effective.Value.Str)
}

func TestRuntimeCollectorAdmitsTheBuildFactsItDoesNotHave(t *testing.T) {
	registry := runtimeRegistry(t, diag.RuntimeDeps{Version: "1.4.2", Mode: "headless"})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	fields := snapshot.Categories[0].Fields

	for _, key := range []string{diag.KeyBuildCommit, diag.KeyBuildDate} {
		field := fieldByKey(t, fields, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Equal(t, diag.KindNone, field.Effective.Value.Kind, key)
		assert.Empty(t, field.Effective.Value.Str, key)
	}
}

func TestRuntimeCollectorReportsDeveloperModeOffAsPresentFalse(t *testing.T) {
	registry := runtimeRegistry(t, diag.RuntimeDeps{Version: "1.4.2", Mode: "headless", Enabled: false})

	explanation, err := registry.Explain(t.Context(), diag.CategoryRuntime, diag.KeyDeveloperMode)
	require.NoError(t, err)
	assert.Equal(t, diag.StateUnset, explanation.Field.Configured.State)
	assert.Equal(t, diag.SourceDefault, explanation.Field.Configured.Source.Kind)
	assert.Equal(t, diag.StatePresent, explanation.Field.Effective.State)
	assert.False(t, explanation.Field.Effective.Value.Bool)
	assert.Contains(t, explanation.JSON(), `"bool": false`, "off is a value, not an absence")
}

func TestRuntimeCollectorReadsAccessorsAtCollectTime(t *testing.T) {
	sessionID := ""
	registry := runtimeRegistry(t, diag.RuntimeDeps{
		Version:   "1.4.2",
		Mode:      "headless",
		SessionID: func() string { return sessionID },
	})

	before, err := registry.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
	require.NoError(t, err)
	assert.Equal(
		t,
		diag.StateUnavailable,
		before.Field.Effective.State,
		"a session that has not started reports nothing",
	)

	sessionID = "sess-9"
	after, err := registry.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
	require.NoError(t, err)
	assert.Equal(t, "sess-9", after.Field.Effective.Value.Str)
}

func TestRuntimeCollectorSurvivesMissingAccessors(t *testing.T) {
	registry := runtimeRegistry(t, diag.RuntimeDeps{})

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	fields := snapshot.Categories[0].Fields
	for _, key := range []string{diag.KeyVersion, diag.KeyMode, diag.KeyWorkspaceRoot, diag.KeySessionID} {
		assert.Equal(t, diag.StateUnavailable, fieldByKey(t, fields, key).Effective.State, key)
	}
}

func TestRuntimeCollectorCollapsesTheWorkspaceHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	registry := runtimeRegistry(t, diag.RuntimeDeps{
		Version:   "1.4.2",
		Mode:      "headless",
		Workspace: func() string { return home + "/src/cozyphi" },
	})

	explanation, err := registry.Explain(t.Context(), diag.CategoryRuntime, diag.KeyWorkspaceRoot)
	require.NoError(t, err)
	assert.Equal(t, "~/src/cozyphi", explanation.Field.Effective.Value.Str)
}

func TestRuntimeCatalogDeclaresExactlyWhatItCanAnswer(t *testing.T) {
	registry := runtimeRegistry(t, diag.RuntimeDeps{Version: "1.4.2", Mode: "headless"})

	entry := registry.Catalog().Categories[0]
	require.Equal(t, diag.CategoryRuntime, entry.Category)
	assert.Equal(t, diag.AvailabilityAvailable, entry.Availability)

	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	keys := make([]string, 0, len(snapshot.Categories[0].Fields))
	for _, field := range snapshot.Categories[0].Fields {
		keys = append(keys, field.Key)
	}
	assert.Equal(t, entry.Keys, keys, "every declared key is answerable and every answer is declared")

	for _, key := range entry.Keys {
		_, err := registry.Explain(t.Context(), diag.CategoryRuntime, key)
		assert.NoError(t, err, key)
	}
}
