package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The diagnostics category is the process observing itself, and every one of
// its owners has to be reachable from a running session rather than only
// from a test's own wiring.
func TestTheDiagnosticsCategoryAnswersAboutThisProcess(t *testing.T) {
	t.Setenv("COZYPHI_PPROF", "")
	c := developerToolRuntime(t)

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryDiagnostics)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	entry := snapshot.Categories[0]
	require.Equal(t, diag.AvailabilityAvailable, entry.Availability, entry.Reason)

	fields := make(map[string]diag.Field, len(entry.Fields))
	for _, field := range entry.Fields {
		fields[field.Key] = field
	}
	for _, key := range []string{
		diag.KeyLoggingState,
		diag.KeyLoggingDestination,
		diag.KeyLoggingSubsystems,
		diag.KeyTelemetryState,
		diag.KeyTelemetryExport,
		diag.KeyProfilingState,
		diag.KeyHarnessLimits,
		diag.KeyHeadlessOutput,
		diag.KeyHeadlessRounds,
		diag.KeyHeadlessTimeout,
	} {
		field, wired := fields[key]
		require.True(t, wired, "the session must reach %s", key)
		assert.NotEqual(t, diag.StateUnavailable, field.Effective.State,
			"%s has an owner and must not read as a wiring gap", key)
	}

	assert.Equal(t, string(diag.TelemetryTracked), fields[diag.KeyTelemetryState].Loaded.Value.Str,
		"a live session holds a tracker")
	assert.Equal(t, string(diag.ProfilingOff), fields[diag.KeyProfilingState].Effective.Value.Str,
		"this process serves no profiles")
	assert.Contains(t, fields[diag.KeyHarnessLimits].Effective.Value.List, "answer_time=5s",
		"the view reports the limits it answers under")
	for _, key := range []string{diag.KeyHeadlessOutput, diag.KeyHeadlessRounds, diag.KeyHeadlessTimeout} {
		assert.Equal(t, diag.StateNotApplicable, fields[key].Effective.State,
			"%s is a flag of cozyphi run and a terminal session was started by none", key)
	}
}

// One process field explained is the same field the snapshot carried, so a
// reader who narrows to it gets provenance rather than a second answer.
func TestAProcessFieldCanBeExplainedOnItsOwn(t *testing.T) {
	c := developerToolRuntime(t)

	explanation, err := c.diagnostics.Explain(t.Context(), diag.CategoryDiagnostics, diag.KeyTelemetryExport)
	require.NoError(t, err)
	assert.Equal(t, diag.KeyTelemetryExport, explanation.Field.Key)
	assert.False(t, explanation.Field.Effective.Value.Bool, "nothing leaves this process")
	assert.NotEmpty(t, explanation.Field.Effective.Source.Ref, "and the answer says how it is known")
}
