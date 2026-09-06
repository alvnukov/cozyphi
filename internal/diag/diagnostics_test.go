package diag_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveWatches is a session that has started every shape of watch and seen
// every way one can end. The gaps between the three layers below are all in
// here: a watch that is live against one that is not, a tight interval that
// has stopped against a slower one that has not, and three kinds of ending.
func liveWatches() diag.WatchState {
	return diag.WatchState{
		Known:          true,
		Managed:        true,
		MaxLive:        8,
		MinInterval:    5 * time.Second,
		FloodLimit:     20,
		FloodWindow:    time.Minute,
		MaxPerDelivery: 5,
		EventTextLimit: 2000,
		Watches: []diag.WatchFacts{
			{Shape: diag.WatchStream, Trigger: diag.WatchOnLine, Events: 12, Live: true, Outcome: diag.WatchRunning},
			{Shape: diag.WatchStream, Trigger: diag.WatchOnExit, Outcome: diag.WatchFailed},
			{Shape: diag.WatchPoll, Every: 30 * time.Second, Events: 3, Live: true, Outcome: diag.WatchRunning},
			{Shape: diag.WatchPoll, Every: 10 * time.Second, Events: 5, Outcome: diag.WatchEnded},
			{Shape: diag.WatchTimer, Every: 2 * time.Minute, Events: 20, Outcome: diag.WatchFlooded},
		},
		Revision: "w5.l2.e40",
	}
}

func watchFields(t *testing.T, state diag.WatchState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewDiagnosticCollector(diag.DiagnosticDeps{Watches: func() diag.WatchState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryDiagnostics)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// "No watch reported anything" is four situations, and only one of them is
// worth acting on.
func TestTheWatchLifecycleSeparatesNoManagerFromNothingRunning(t *testing.T) {
	tests := []struct {
		name  string
		state func(diag.WatchState) diag.WatchState
		want  diag.WatchLifecycle
	}{
		{
			name:  "a process shape that builds no manager",
			state: func(s diag.WatchState) diag.WatchState { s.Managed = false; return s },
			want:  diag.WatchesNoManager,
		},
		{
			name:  "a manager with nothing running",
			state: func(s diag.WatchState) diag.WatchState { s.Watches = nil; return s },
			want:  diag.WatchesIdle,
		},
		{
			name:  "something running with room for another",
			state: func(s diag.WatchState) diag.WatchState { return s },
			want:  diag.WatchesRunning,
		},
		{
			name:  "every live slot taken",
			state: func(s diag.WatchState) diag.WatchState { s.MaxLive = 2; return s },
			want:  diag.WatchesFull,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := fieldByKey(t, watchFields(t, tc.state(liveWatches())), diag.KeyWatchesState)
			assert.Equal(t, string(tc.want), state.Effective.Value.Str)
		})
	}
}

// Carrying watches and holding a manager are two answers. Nothing in the
// configuration switches watches off, so a session with none is a fact about
// the shape the process was started in.
func TestABuildThatCarriesWatchesIsNotAProcessThatHoldsOne(t *testing.T) {
	on := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesState)
	assert.Equal(t, string(diag.WatchesSupported), on.Configured.Value.Str)
	assert.Equal(t, diag.SourceBuild, on.Configured.Source.Kind)
	assert.Contains(t, on.Configured.Source.Ref, "nothing in the configuration switches them on or off")
	assert.Equal(t, string(diag.WatchesManaged), on.Loaded.Value.Str)

	headless := liveWatches()
	headless.Managed = false
	field := fieldByKey(t, watchFields(t, headless), diag.KeyWatchesState)
	assert.Equal(t, string(diag.WatchesSupported), field.Configured.Value.Str,
		"the build is unchanged; what is missing is the manager this run never built")
	assert.Equal(t, string(diag.WatchesNoManager), field.Loaded.Value.Str)
}

// A refused start is answered by the last line of this field, not the first:
// the budgets are compiled in, and what a reader wants is how much is left.
func TestTheBudgetsAreReportedWithWhatIsLeftOfThem(t *testing.T) {
	limits := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesLimits)

	assert.Equal(t, []string{
		"live=8", "interval_floor=5s", "events=20/1m0s", "per_delivery=5", "event_text=2000",
	}, limits.Configured.Value.List)
	assert.Equal(t, diag.SourceBuild, limits.Configured.Source.Kind)
	assert.Equal(t, diag.StateNotApplicable, limits.Loaded.State,
		"no configuration file, environment entry or session setting loads over a compiled-in budget")
	assert.Equal(t, int64(6), limits.Effective.Value.Int, "eight slots less the two in use")
	assert.Equal(t, diag.ScopeProcess, limits.Scope)

	headless := liveWatches()
	headless.Managed = false
	assert.Equal(t, diag.StateNotApplicable,
		fieldByKey(t, watchFields(t, headless), diag.KeyWatchesLimits).Effective.State,
		"there is no slot to be free of when nothing can start a watch")
	assert.NotEmpty(t,
		fieldByKey(t, watchFields(t, headless), diag.KeyWatchesLimits).Configured.Value.List,
		"the budgets still hold wherever a manager would be built")
}

// A watch is counted by what triggers it, because a streaming command that
// reports on exit stays quiet until it finishes — and quiet is exactly the
// symptom that sends someone here.
func TestAStreamIsCountedByWhatTriggersItRatherThanByShapeAlone(t *testing.T) {
	shapes := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesShapes)

	assert.Equal(t, []string{"stream:line", "stream:exit", "poll", "timer"}, shapes.Configured.Value.List)
	assert.Equal(t, []string{"stream:line=1", "stream:exit=1", "poll=2", "timer=1"}, shapes.Loaded.Value.List)
	assert.Equal(t, []string{"stream:line=1", "poll=1"}, shapes.Effective.Value.List,
		"only the ones still running")
	assert.Contains(t, shapes.Loaded.Source.Ref, "no label, command, match expression or ")
}

// A stream carries no interval at all, so a session running only streams
// answers that it ticks on nothing rather than answering zero.
func TestTheCadenceReportedIsTheOneStillBeingPaidFor(t *testing.T) {
	cadence := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesCadence)

	assert.Equal(t, "5s", cadence.Configured.Value.Str)
	assert.Equal(t, diag.SourceBuild, cadence.Configured.Source.Kind)
	assert.Equal(t, "10s", cadence.Loaded.Value.Str, "the tightest this session ever ran")
	assert.Equal(t, "30s", cadence.Effective.Value.Str, "and the tightest still running")

	streamsOnly := liveWatches()
	streamsOnly.Watches = []diag.WatchFacts{
		{Shape: diag.WatchStream, Trigger: diag.WatchOnLine, Live: true, Outcome: diag.WatchRunning},
	}
	field := fieldByKey(t, watchFields(t, streamsOnly), diag.KeyWatchesCadence)
	assert.Equal(t, diag.StateNotApplicable, field.Loaded.State)
	assert.Contains(t, field.Loaded.Source.Ref, "reports when its output does, not on a clock")
}

// How much a watch has had to say, never what it said.
func TestEventsAreCountedAndNeverQuoted(t *testing.T) {
	events := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesEvents)

	assert.Equal(t, diag.StateNotApplicable, events.Configured.State,
		"an event happens because the thing being watched did, not because a file asked for it")
	assert.Equal(t, int64(40), events.Loaded.Value.Int)
	assert.Equal(t, int64(15), events.Effective.Value.Int, "from the two still running")
	assert.Contains(t, events.Loaded.Source.Ref, "How many, never what")
}

// A command that failed and a watch that talked itself over the session's
// event budget are different problems with different fixes, and neither is a
// watch that simply finished.
func TestAFailedWatchAndAFloodedOneAreDifferentAnswers(t *testing.T) {
	outcomes := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesOutcomes)

	assert.Equal(t, []string{"running", "ended", "failed", "flooded"}, outcomes.Configured.Value.List)
	assert.Equal(t, []string{"running=2", "ended=1", "failed=1", "flooded=1"}, outcomes.Loaded.Value.List)
	assert.Equal(t, int64(2), outcomes.Effective.Value.Int, "the failed one and the flooded one")
	assert.Contains(t, outcomes.Effective.Source.Ref, "a watch's error quotes the command it ran",
		"the count is reported and the reason deliberately is not")
}

// A run with no watch manager is not a session that has started no watch,
// and reporting it as an empty list would read as one.
func TestAHeadlessRunReportsNoManagerRatherThanAnEmptySession(t *testing.T) {
	headless := liveWatches()
	headless.Managed = false

	perSession := []string{
		diag.KeyWatchesCount,
		diag.KeyWatchesShapes,
		diag.KeyWatchesCadence,
		diag.KeyWatchesEvents,
		diag.KeyWatchesOutcomes,
	}
	for _, key := range perSession {
		field := fieldByKey(t, watchFields(t, headless), key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Equal(t, diag.StateNotApplicable, field.Loaded.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "a headless run has none", key)
	}
}

// A layer nobody wired knows nothing, which is a third answer again: not a
// missing manager and not an empty session.
func TestAnUnwiredWatchLayerIsUnavailableEverywhere(t *testing.T) {
	for _, field := range watchFields(t, diag.WatchState{}) {
		assert.Equal(t, diag.StateUnavailable, field.Configured.State, field.Key)
		assert.Equal(t, diag.StateUnavailable, field.Loaded.State, field.Key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, field.Key)
	}
}

// The catalog is answered from the declared key set alone, and no key
// addresses one watch: a key per watch would make the catalog depend on what
// a turn started, and would address one by a label this view never reports.
func TestTheWatchCatalogIsStaticAndAddressesNoSingleWatch(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewDiagnosticCollector(diag.DiagnosticDeps{Watches: liveWatches}))

	var keys []string
	for _, entry := range registry.Catalog().Categories {
		if entry.Category == diag.CategoryDiagnostics {
			keys = entry.Keys
		}
	}
	require.Equal(t, []string{
		diag.KeyWatchesState,
		diag.KeyWatchesLimits,
		diag.KeyWatchesCount,
		diag.KeyWatchesShapes,
		diag.KeyWatchesCadence,
		diag.KeyWatchesEvents,
		diag.KeyWatchesOutcomes,
	}, keys)

	for _, key := range keys {
		explained, err := registry.Explain(t.Context(), diag.CategoryDiagnostics, key)
		require.NoError(t, err, key)
		assert.Equal(t, key, explained.Field.Key)
	}
	_, err := registry.Explain(t.Context(), diag.CategoryDiagnostics, "watches.watch.build")
	assert.Error(t, err, "no key addresses one watch")
}

// How many this session started and how many are still running; the gap
// between them is what the outcomes field breaks down.
func TestTheWatchCountSeparatesStartedFromStillRunning(t *testing.T) {
	count := fieldByKey(t, watchFields(t, liveWatches()), diag.KeyWatchesCount)

	assert.Equal(t, diag.StateNotApplicable, count.Configured.State)
	assert.Equal(t, int64(5), count.Loaded.Value.Int)
	assert.Equal(t, int64(2), count.Effective.Value.Int)
	assert.Equal(t, "w5.l2.e40", count.Revision,
		"the fingerprint travels with the field, so two snapshots across a start are visibly different")
}
