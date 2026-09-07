package plantel_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plantel"
)

// A session without a tracker is telemetry turned off, which is what every
// other method here takes a nil tracker for. It is not a wiring gap, and the
// observation says which of the two it is.
func TestASessionWithoutATrackerIsTelemetryTurnedOff(t *testing.T) {
	facts := plantel.Observe(nil)

	assert.True(t, facts.Known, "the layer answered; it just has no tracker to report")
	assert.False(t, facts.Tracked)
	assert.Empty(t, facts.Readable, "nothing is counting, so there is nothing to read anywhere")
	assert.Equal(t, reflect.TypeFor[plantel.Snapshot]().NumField(), facts.Counters,
		"the surface is as wide as the build makes it, tracker or no tracker")
}

// With a tracker in force the counters exist and can be read in exactly one
// place. Naming it is the point: a reader asking where telemetry goes gets
// an answer instead of an assumption.
func TestATrackedSessionSaysWhereTheCountersCanBeRead(t *testing.T) {
	facts := plantel.Observe(&plantel.Tracker{})

	assert.True(t, facts.Tracked)
	require.Len(t, facts.Readable, 1, "one place, and it is inside this process")
	assert.Contains(t, facts.Readable[0], "plan category")
	assert.NotEqual(t, plantel.Observe(nil).Revision, facts.Revision,
		"a session that counts and one that does not are two different states")
}

// The observation is about the mechanism, not the numbers. Asking must not
// record anything, reset anything or disturb what the counters say — what
// they say is the plan category's answer and is read there.
func TestObservingTheTrackerCountsNothingAndDisturbsNothing(t *testing.T) {
	tracker := &plantel.Tracker{}
	tracker.Miss()
	tracker.PatchRetry()
	tracker.ProjectionBytes(4096)
	before := tracker.Snapshot()

	facts := plantel.Observe(tracker)

	assert.Equal(t, before, tracker.Snapshot(), "observing is not a record")
	assert.Equal(t, reflect.TypeFor[plantel.Snapshot]().NumField(), facts.Counters,
		"the width of the schema is a build fact and not a count of anything recorded")
}

// The fixed, numeric-only schema is the leak contract, and this is where the
// harness view inherits it: there is no string, map or slice field for plan
// text, evidence or a label to be reported through.
func TestTheObservedSurfaceHasNowhereForPlanTextToTravel(t *testing.T) {
	shape := reflect.TypeFor[plantel.Snapshot]()
	for field := range shape.Fields() {
		assert.Equal(t, reflect.Uint64, field.Type.Kind(), "field %s is numeric", field.Name)
	}
	assert.Equal(t, shape.NumField(), plantel.Observe(&plantel.Tracker{}).Counters,
		"and the harness reports that whole surface rather than a subset of it")
}
