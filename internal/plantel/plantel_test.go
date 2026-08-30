package plantel_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plantel"
)

// TestTrackerNilReceiverIsOff pins the degrade contract: telemetry without a
// configured tracker is simply off. Every record call is a no-op and the
// snapshot reads as zero instead of panicking, so callers never need an
// if-configured dance around the seam.
func TestTrackerNilReceiverIsOff(t *testing.T) {
	var tracker *plantel.Tracker
	tracker.Miss()
	tracker.MaterialRevision()
	tracker.ApprovalChurn()
	tracker.TransitionConflict()
	tracker.IdempotentRetry()
	tracker.StandaloneStart()
	tracker.PlanOnlyRound()
	tracker.ProjectionBytes(1024)
	tracker.CompletionWithoutEvidence()
	tracker.ArchiveLatency(time.Second)
	tracker.DraftCreated(plantel.AuthoringLegacy)
	tracker.ApprovalLatency(time.Second)
	tracker.MaterialReapproval()
	tracker.PatchRetry()
	tracker.CompletionSuccess()
	tracker.CompletionAbandoned()
	assert.Equal(t, plantel.Snapshot{}, tracker.Snapshot(), "a nil tracker must read as zero")
}

// TestSnapshotCountersAreIndependent is the isolation contract: each record
// moves exactly its own counter and leaves every other field untouched.
func TestSnapshotCountersAreIndependent(t *testing.T) {
	records := []struct {
		name  string
		moved func(*plantel.Tracker)
		read  func(plantel.Snapshot) uint64
	}{
		{
			"miss", func(tr *plantel.Tracker) { tr.Miss() },
			func(s plantel.Snapshot) uint64 { return s.PlanMisses },
		},
		{
			"material revision", func(tr *plantel.Tracker) { tr.MaterialRevision() },
			func(s plantel.Snapshot) uint64 { return s.MaterialRevisions },
		},
		{
			"approval churn", func(tr *plantel.Tracker) { tr.ApprovalChurn() },
			func(s plantel.Snapshot) uint64 { return s.ApprovalChurn },
		},
		{
			"transition conflict", func(tr *plantel.Tracker) { tr.TransitionConflict() },
			func(s plantel.Snapshot) uint64 { return s.TransitionConflicts },
		},
		{
			"idempotent retry", func(tr *plantel.Tracker) { tr.IdempotentRetry() },
			func(s plantel.Snapshot) uint64 { return s.IdempotentRetries },
		},
		{
			"standalone start", func(tr *plantel.Tracker) { tr.StandaloneStart() },
			func(s plantel.Snapshot) uint64 { return s.StandaloneStarts },
		},
		{
			"plan-only round", func(tr *plantel.Tracker) { tr.PlanOnlyRound() },
			func(s plantel.Snapshot) uint64 { return s.PlanOnlyRounds },
		},
		{
			"completion without evidence", func(tr *plantel.Tracker) { tr.CompletionWithoutEvidence() },
			func(s plantel.Snapshot) uint64 { return s.CompletionsWithoutEvidence },
		},
		{
			"draft adaptive", func(tr *plantel.Tracker) { tr.DraftCreated(plantel.AuthoringAdaptive) },
			func(s plantel.Snapshot) uint64 { return s.DraftsAdaptive },
		},
		{
			"draft legacy", func(tr *plantel.Tracker) { tr.DraftCreated(plantel.AuthoringLegacy) },
			func(s plantel.Snapshot) uint64 { return s.DraftsLegacy },
		},
		{
			"material reapproval", func(tr *plantel.Tracker) { tr.MaterialReapproval() },
			func(s plantel.Snapshot) uint64 { return s.MaterialReapprovals },
		},
		{
			"patch retry", func(tr *plantel.Tracker) { tr.PatchRetry() },
			func(s plantel.Snapshot) uint64 { return s.PatchRetries },
		},
		{
			"completion success", func(tr *plantel.Tracker) { tr.CompletionSuccess() },
			func(s plantel.Snapshot) uint64 { return s.CompletionsSuccess },
		},
		{
			"completion abandoned", func(tr *plantel.Tracker) { tr.CompletionAbandoned() },
			func(s plantel.Snapshot) uint64 { return s.CompletionsAbandoned },
		},
	}
	for _, record := range records {
		t.Run(record.name, func(t *testing.T) {
			var tracker plantel.Tracker
			for _, other := range records {
				if other.name != record.name {
					other.moved(&tracker)
				}
			}
			before := tracker.Snapshot()
			record.moved(&tracker)
			after := tracker.Snapshot()
			assert.EqualValues(t, 1, record.read(after), "the record must move its own counter")
			record.read(before) // keep before alive for the diff below
			assert.EqualValues(t, 0, record.read(before), "the counter starts at zero")
			for _, other := range records {
				if other.name != record.name {
					assert.EqualValues(t, 1, other.read(after),
						"%s must not move %s", record.name, other.name)
				}
			}
		})
	}
}

// TestProjectionBytesAggregates pins the byte accounting: one record sets the
// last-injection size and adds it to the cumulative total, so the snapshot can
// answer both "how big was the prompt hit" and "how much did we spend".
func TestProjectionBytesAggregates(t *testing.T) {
	var tracker plantel.Tracker
	tracker.ProjectionBytes(100)
	s := tracker.Snapshot()
	assert.EqualValues(t, 1, s.ProjectionInjections)
	assert.EqualValues(t, 100, s.ProjectionBytesLast)
	assert.EqualValues(t, 100, s.ProjectionBytes)

	tracker.ProjectionBytes(40)
	s = tracker.Snapshot()
	assert.EqualValues(t, 2, s.ProjectionInjections)
	assert.EqualValues(t, 40, s.ProjectionBytesLast)
	assert.EqualValues(t, 140, s.ProjectionBytes)
}

// TestArchiveLatencyBounds pins the duration contract: latency is recorded in
// bounded milliseconds with a running max, never as a raw timestamp or string.
func TestArchiveLatencyBounds(t *testing.T) {
	var tracker plantel.Tracker
	tracker.ArchiveLatency(2 * time.Millisecond)
	tracker.ArchiveLatency(5 * time.Millisecond)
	s := tracker.Snapshot()
	assert.EqualValues(t, 5, s.ArchiveLatencyLastMS)
	assert.EqualValues(t, 5, s.ArchiveLatencyMaxMS)
	assert.EqualValues(t, 2, s.Archives, "each latency record is one archive")
}

// TestSnapshotSchemaIsFixedAndBounded is the leak contract: the snapshot is
// the entire telemetry surface, and every field is a uint64. No string, map
// or slice field can exist, so no plan text, tool output, secret or unbounded
// label can ever ride along, and the JSON schema is exactly the struct.
func TestSnapshotSchemaIsFixedAndBounded(t *testing.T) {
	typ := reflect.TypeFor[plantel.Snapshot]()
	require.NotNil(t, typ)
	for field := range typ.Fields() {
		assert.Equalf(t, reflect.Uint64, field.Type.Kind(),
			"field %s must be uint64, is %s", field.Name, field.Type)
	}
	blob, err := json.Marshal(plantel.Snapshot{})
	require.NoError(t, err)
	var keys map[string]any
	require.NoError(t, json.Unmarshal(blob, &keys))
	assert.Len(t, keys, typ.NumField(), "json keys must be exactly the fixed struct schema")
}

// TestDraftsTagByAuthoringPolicy pins the policy tag: the draft counter is a
// fixed pair, not a label — the closed authoring_policy selector decides
// which of the two counters moves, and nothing textual enters the snapshot.
func TestDraftsTagByAuthoringPolicy(t *testing.T) {
	var tracker plantel.Tracker
	tracker.DraftCreated(plantel.AuthoringAdaptive)
	tracker.DraftCreated(plantel.AuthoringAdaptive)
	tracker.DraftCreated(plantel.AuthoringLegacy)
	s := tracker.Snapshot()
	assert.EqualValues(t, 2, s.DraftsAdaptive)
	assert.EqualValues(t, 1, s.DraftsLegacy)
}

// TestApprovalLatencyBucketsAreBounded pins the latency contract: one
// decision lands in exactly one fixed bucket, boundaries are inclusive, and
// clock skew clamps into the fastest bucket instead of wrapping a counter.
func TestApprovalLatencyBucketsAreBounded(t *testing.T) {
	var tracker plantel.Tracker
	tracker.ApprovalLatency(500 * time.Millisecond)
	tracker.ApprovalLatency(time.Second)
	tracker.ApprovalLatency(5 * time.Second)
	tracker.ApprovalLatency(30 * time.Second)
	tracker.ApprovalLatency(90 * time.Second)
	tracker.ApprovalLatency(-time.Second)
	s := tracker.Snapshot()
	assert.EqualValues(
		t,
		3,
		s.ApprovalLatency1s,
		"500ms, the 1s boundary and the clock-skew clamp land in the first bucket",
	)
	assert.EqualValues(t, 1, s.ApprovalLatency10s)
	assert.EqualValues(t, 1, s.ApprovalLatency60s)
	assert.EqualValues(t, 1, s.ApprovalLatencySlow)
}
