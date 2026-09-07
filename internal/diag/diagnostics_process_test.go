package diag_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// processFields is the diagnostics category answered from whatever owners a
// test wired, so a field about the process can be read without a process.
func processFields(t *testing.T, deps diag.DiagnosticDeps) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), diag.NewDiagnosticCollector(deps))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryDiagnostics)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

func loggingDeps(facts diag.LoggingFacts) diag.DiagnosticDeps {
	return diag.DiagnosticDeps{Logging: func() diag.LoggingFacts { return facts }}
}

// The switch is read once and remembered. An environment changed afterwards
// is configured one way and acting another, and that gap is the usual
// explanation for a debug log that is "on" and empty.
func TestTheDebugSwitchAndWhatItLatchedToAreTwoAnswers(t *testing.T) {
	turnedOnTooLate := diag.LoggingFacts{Known: true, Requested: true, Latched: true, Enabled: false}
	field := fieldByKey(t, processFields(t, loggingDeps(turnedOnTooLate)), diag.KeyLoggingState)

	assert.True(t, field.Configured.Value.Bool, "the environment says on")
	assert.Equal(t, diag.SourceEnv, field.Configured.Source.Kind)
	assert.False(t, field.Loaded.Value.Bool, "and the answer it was read into says off")
	assert.False(t, field.Effective.Value.Bool, "so a line written now would not land")
	assert.Contains(t, field.Loaded.Source.Ref, "read once and remembered")
	assert.Equal(t, diag.ApplyRestart, field.Apply)

	unread := diag.LoggingFacts{Known: true, Requested: true}
	early := fieldByKey(t, processFields(t, loggingDeps(unread)), diag.KeyLoggingState)
	assert.Equal(t, diag.StateUnset, early.Loaded.State, "nothing has latched it yet")
	assert.True(t, early.Effective.Value.Bool,
		"a line written now would latch it on, so on is what it would do")
}

// Only the name travels. Where the file sits is a home directory or a path a
// script chose, and neither is needed to answer the question.
func TestTheLogDestinationIsANameAndNeverADirectory(t *testing.T) {
	named := diag.LoggingFacts{
		Known:       true,
		Destination: "cozyphi-debug.log",
		FromEnv:     true,
		OpenName:    "cozyphi-debug.log",
	}
	field := fieldByKey(t, processFields(t, loggingDeps(named)), diag.KeyLoggingDestination)

	assert.Equal(t, "cozyphi-debug.log", field.Configured.Value.Str)
	assert.Equal(t, diag.SourceEnv, field.Configured.Source.Kind)
	assert.Contains(t, field.Configured.Source.Ref, "the directory it sits in is not reported")
	assert.Equal(t, "cozyphi-debug.log", field.Effective.Value.Str)

	quiet := diag.LoggingFacts{Known: true, Destination: "cozyphi-debug.log"}
	nothingOpen := fieldByKey(t, processFields(t, loggingDeps(quiet)), diag.KeyLoggingDestination)
	assert.Equal(t, diag.StateNotApplicable, nothingOpen.Loaded.State,
		"a log with nothing to say has a destination and no open file")
	assert.Contains(t, nothingOpen.Loaded.Source.Ref, "opened by the first line")
}

func telemetryDeps(facts diag.TelemetryFacts) diag.DiagnosticDeps {
	return diag.DiagnosticDeps{Telemetry: func() diag.TelemetryFacts { return facts }}
}

// The mechanism, not the numbers: what the counters say is the plan
// category's answer, and two answers to one question is one too many.
func TestTelemetryReportsTheMechanismAndNotTheCounters(t *testing.T) {
	tracked := diag.TelemetryFacts{Known: true, Tracked: true, Counters: 24}
	field := fieldByKey(t, processFields(t, telemetryDeps(tracked)), diag.KeyTelemetryState)

	assert.Equal(t, string(diag.TelemetrySupported), field.Configured.Value.Str)
	assert.Equal(t, diag.SourceBuild, field.Configured.Source.Kind)
	assert.Equal(t, string(diag.TelemetryTracked), field.Loaded.Value.Str)
	assert.Equal(t, int64(24), field.Effective.Value.Int)
	assert.Contains(t, field.Effective.Source.Ref, "numeric by construction")

	untracked := diag.TelemetryFacts{Known: true, Counters: 24}
	off := fieldByKey(t, processFields(t, telemetryDeps(untracked)), diag.KeyTelemetryState)
	assert.Equal(t, string(diag.TelemetrySupported), off.Configured.Value.Str,
		"the build still carries the mechanism; this session holds no tracker")
	assert.Equal(t, string(diag.TelemetryUntracked), off.Loaded.Value.Str)
	assert.Equal(t, int64(0), off.Effective.Value.Int, "nothing is counting")
}

// Telemetry is the part of a harness a reader is entitled to assume phones
// home. Here it does not, and that is worth a field rather than a footnote.
func TestNothingExportsTheCounters(t *testing.T) {
	tracked := diag.TelemetryFacts{Known: true, Tracked: true, Counters: 24, Readable: []string{"harness view"}}
	field := fieldByKey(t, processFields(t, telemetryDeps(tracked)), diag.KeyTelemetryExport)

	assert.Equal(t, diag.StateNotApplicable, field.Configured.State)
	assert.Contains(t, field.Configured.Source.Ref, "no metrics endpoint")
	assert.Equal(t, []string{"harness view"}, field.Loaded.Value.List)
	assert.False(t, field.Effective.Value.Bool, "nothing leaves this process")
	assert.Contains(t, field.Effective.Source.Ref, "never persisted and are never sent anywhere")
}

func profilingDeps(facts diag.ProfilingFacts) diag.DiagnosticDeps {
	return diag.DiagnosticDeps{Profiling: func() diag.ProfilingFacts { return facts }}
}

// How exposed the endpoint is is the question worth answering. The host and
// the port are not needed to answer it and never travel.
func TestProfilingReportsHowFarItReachesAndNeverTheAddress(t *testing.T) {
	serving := diag.ProfilingFacts{
		Known:     true,
		Requested: true,
		Exposure:  diag.ProfilingEveryInterface,
		Lifecycle: diag.ProfilingServing,
	}
	fields := processFields(t, profilingDeps(serving))
	field := fieldByKey(t, fields, diag.KeyProfilingState)

	assert.True(t, field.Configured.Value.Bool)
	assert.Equal(t, string(diag.ProfilingEveryInterface), field.Loaded.Value.Str)
	assert.Equal(t, string(diag.ProfilingServing), field.Effective.Value.Str)
	assert.Equal(t, diag.ApplyRestart, field.Apply, "the environment is read at start and nowhere else")

	rendered := renderFields(fields)
	for _, forbidden := range []string{"6060", "0.0.0.0", "127.0.0.1", "localhost", "http://"} {
		assert.NotContains(t, rendered, forbidden, "no part of the address may travel")
	}
}

// An endpoint being up is not a capability. The read-only view comes from
// the command line and from nothing else, and the field says so where
// somebody reasoning about the endpoint will read it.
func TestAProfilingEndpointGrantsNothing(t *testing.T) {
	serving := diag.ProfilingFacts{
		Known: true, Requested: true, Exposure: diag.ProfilingLoopback, Lifecycle: diag.ProfilingServing,
	}
	field := fieldByKey(t, processFields(t, profilingDeps(serving)), diag.KeyProfilingState)
	assert.Contains(t, field.Effective.Source.Ref, "--developer-mode")
	assert.Contains(t, field.Effective.Source.Ref, "grants nothing")

	askedForLate := diag.ProfilingFacts{Known: true, Requested: true, Lifecycle: diag.ProfilingOff}
	late := fieldByKey(t, processFields(t, profilingDeps(askedForLate)), diag.KeyProfilingState)
	assert.True(t, late.Configured.Value.Bool, "the environment names one now")
	assert.Equal(t, diag.StateNotApplicable, late.Loaded.State)
	assert.Contains(t, late.Loaded.Source.Ref, "read once, at start")
	assert.Equal(t, string(diag.ProfilingOff), late.Effective.Value.Str, "and nothing is listening")
}

// A truncated answer says it was truncated; this is where a reader finds out
// at what. The limits acting are not always the limits asked for.
func TestTheViewReportsTheLimitsItAnswersUnder(t *testing.T) {
	asked := diag.Limits{MaxValueBytes: 64}
	field := fieldByKey(t, processFields(t, diag.DiagnosticDeps{Response: asked}), diag.KeyHarnessLimits)

	assert.Contains(t, field.Configured.Value.List, "value_bytes=512", "what every caller starts from")
	assert.Contains(t, field.Loaded.Value.List, "value_bytes=64", "what this view was asked for")
	assert.Contains(t, field.Effective.Value.List, "value_bytes=64")
	assert.Contains(t, field.Effective.Value.List, "total_bytes=16384",
		"an unset limit falls back to its default rather than being unbounded")
	assert.NotEmpty(t, field.Revision, "the limits in force fingerprint the answer taken under them")
}

// renderFields is every string one answer carries, so a test can assert that
// something is absent from all of it rather than from one member it guessed.
func renderFields(fields []diag.Field) string {
	var b strings.Builder
	for _, field := range fields {
		b.WriteString(field.Key)
		for _, observation := range []diag.Observation{field.Configured, field.Loaded, field.Effective} {
			b.WriteString(" " + string(observation.State))
			b.WriteString(" " + observation.Value.Str)
			b.WriteString(" " + strings.Join(observation.Value.List, " "))
			b.WriteString(" " + observation.Source.Ref)
		}
		b.WriteString(" " + field.Revision + "\n")
	}
	return b.String()
}
