package diag_test

import (
	"strings"
	"testing"
	"time"

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

// Which subsystem logs the environment redirects is the fact; where to is a
// path a person chose. A planted path proves it goes nowhere.
func TestARedirectedSubsystemLogIsNamedAndItsDirectoryIsNot(t *testing.T) {
	redirected := diag.LoggingFacts{Known: true, MCPLogFromEnv: true, PlanGateLogFromEnv: true}
	fields := processFields(t, loggingDeps(redirected))
	field := fieldByKey(t, fields, diag.KeyLoggingSubsystems)

	assert.Equal(t, []string{diag.LogSubsystemMCP, diag.LogSubsystemPlanGate}, field.Configured.Value.List)
	assert.Equal(t, diag.SourceEnv, field.Configured.Source.Kind)
	assert.Contains(t, field.Configured.Source.Ref, "COZYPHI_MCP_LOG_DIR")
	assert.Contains(t, field.Configured.Source.Ref, "COZYPHI_PLAN_GATE_LOG_DIR")
	assert.Contains(t, field.Configured.Source.Ref, "not reported")
	assert.Equal(t, field.Configured.Value.List, field.Effective.Value.List,
		"each directory is read when its log opens, so what the environment says now is what acts")
	assert.Equal(t, diag.ApplyRestart, field.Apply)
	assert.NotContains(t, renderFields(fields), "/", "no directory, and so no separator, may travel")

	one := diag.LoggingFacts{Known: true, PlanGateLogFromEnv: true}
	only := fieldByKey(t, processFields(t, loggingDeps(one)), diag.KeyLoggingSubsystems)
	assert.Equal(t, []string{diag.LogSubsystemPlanGate}, only.Effective.Value.List)
}

// Nothing redirected is a default and not a redirect to nowhere: the answer
// names the layer the defaults come from rather than an empty list alone.
func TestNothingRedirectedIsReportedAsTheDefaultAndNotAsAnEmptyChoice(t *testing.T) {
	quiet := diag.LoggingFacts{Known: true}
	field := fieldByKey(t, processFields(t, loggingDeps(quiet)), diag.KeyLoggingSubsystems)

	assert.Equal(t, diag.StateUnset, field.Configured.State)
	assert.Equal(t, diag.SourceDefault, field.Configured.Source.Kind)
	assert.Empty(t, field.Effective.Value.List)
	assert.Equal(t, diag.StatePresent, field.Effective.State, "an empty list is still the answer in force")
}

func headlessDeps(facts diag.HeadlessFacts) diag.DiagnosticDeps {
	return diag.DiagnosticDeps{Headless: func() diag.HeadlessFacts { return facts }}
}

// The ceilings a headless run was started under are the flags it was given.
// They bind the whole run, so what was asked for is what acts.
func TestAHeadlessRunReportsTheCeilingsItWasStartedUnder(t *testing.T) {
	run := diag.HeadlessFacts{Known: true, Run: true, JSONL: true, MaxRounds: 3, Timeout: 20 * time.Second}
	fields := processFields(t, headlessDeps(run))

	output := fieldByKey(t, fields, diag.KeyHeadlessOutput)
	assert.Equal(t, "jsonl", output.Configured.Value.Str)
	assert.Equal(t, diag.SourceCLIFlag, output.Configured.Source.Kind)
	assert.Contains(t, output.Configured.Source.Ref, "--jsonl")
	assert.Equal(t, "jsonl", output.Effective.Value.Str)

	rounds := fieldByKey(t, fields, diag.KeyHeadlessRounds)
	assert.Equal(t, int64(3), rounds.Configured.Value.Int)
	assert.Contains(t, rounds.Configured.Source.Ref, "--max-rounds")
	assert.Equal(t, int64(3), rounds.Effective.Value.Int)
	assert.Equal(t, diag.SourceSession, rounds.Effective.Source.Kind)

	timeout := fieldByKey(t, fields, diag.KeyHeadlessTimeout)
	assert.Equal(t, "20s", timeout.Configured.Value.Str)
	assert.Contains(t, timeout.Configured.Source.Ref, "--timeout")
	assert.Equal(t, int64(20*time.Second), timeout.Effective.Value.Int)
	for _, field := range []diag.Field{output, rounds, timeout} {
		assert.Equal(t, diag.ApplyRestart, field.Apply, field.Key)
		assert.Equal(t, diag.ScopeProcess, field.Scope, field.Key)
	}
}

// A ceiling nobody asked for is reported as nothing asked. The engine has a
// budget of its own for the rounds, and this view does not read the engine
// to repeat it: a zero here would read as a run that may take no rounds.
func TestACeilingNobodyAskedForIsUnsetAndNotZero(t *testing.T) {
	bare := diag.HeadlessFacts{Known: true, Run: true}
	fields := processFields(t, headlessDeps(bare))

	assert.Equal(t, "text", fieldByKey(t, fields, diag.KeyHeadlessOutput).Effective.Value.Str)
	assert.Equal(t, diag.SourceDefault, fieldByKey(t, fields, diag.KeyHeadlessOutput).Configured.Source.Kind)
	rounds := fieldByKey(t, fields, diag.KeyHeadlessRounds)
	assert.Equal(t, diag.StateUnset, rounds.Configured.State)
	assert.Equal(t, diag.StateUnset, rounds.Effective.State)
	assert.Contains(t, rounds.Effective.Source.Ref, "compiled-in round budget")
	timeout := fieldByKey(t, fields, diag.KeyHeadlessTimeout)
	assert.Equal(t, diag.StateUnset, timeout.Effective.State)
	assert.Contains(t, timeout.Effective.Source.Ref, "no deadline")
}

// A terminal session was started by no run flags. It says so on every layer
// rather than reporting the ceilings as zeros somebody set, and a process
// that wired no observer at all is told apart from both.
func TestATerminalSessionHasNoHeadlessCeilingsRatherThanZeroOnes(t *testing.T) {
	surface := diag.HeadlessFacts{Known: true}
	for _, key := range []string{diag.KeyHeadlessOutput, diag.KeyHeadlessRounds, diag.KeyHeadlessTimeout} {
		field := fieldByKey(t, processFields(t, headlessDeps(surface)), key)
		assert.Equal(t, diag.StateNotApplicable, field.Configured.State, key)
		assert.Equal(t, diag.StateNotApplicable, field.Loaded.State, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "cozyphi run", key)

		unwired := fieldByKey(t, processFields(t, diag.DiagnosticDeps{}), key)
		assert.Equal(t, diag.StateUnavailable, unwired.Effective.State, key)
	}
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
	assert.Contains(t, field.Effective.Value.List, "total_bytes=32768",
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
