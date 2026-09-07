package plantel

import (
	"reflect"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// readableHere names where the counters can be read inside this process. It
// is the harness's own plan category and nothing else: there is no metrics
// endpoint, no collector address and no telemetry file to name beside it.
const readableHere = "harness view, plan category"

// Observe reports plan telemetry for the harness view: whether a tracker is
// in force, how wide the counting surface is, and where the counters can be
// read. It is about the mechanism, not the numbers — what the counters say
// is the plan category's answer and is not repeated here.
//
// A nil tracker is telemetry switched off, which is the same thing every
// other method here takes it for. Nothing is recorded, reset or exported by
// asking, and the tracker's own lock is never taken: the width of the schema
// is a build fact and needs no snapshot to read.
func Observe(t *Tracker) diag.TelemetryFacts {
	facts := diag.TelemetryFacts{
		Known:    true,
		Tracked:  t != nil,
		Counters: reflect.TypeFor[Snapshot]().NumField(),
	}
	if facts.Tracked {
		facts.Readable = []string{readableHere}
	}
	facts.Revision = telemetryRevision(facts)
	return facts
}

// telemetryRevision fingerprints what this observation describes: whether a
// tracker is in force and how wide the surface is. Neither changes during a
// session, so two answers that differ are two sessions.
func telemetryRevision(facts diag.TelemetryFacts) string {
	tracked := "0"
	if facts.Tracked {
		tracked = "1"
	}
	return "t" + tracked + ".n" + strconv.Itoa(facts.Counters)
}
