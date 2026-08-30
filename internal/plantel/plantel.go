// Package plantel is the bounded observability budget for the durable plan:
// one deep module every plan participant records through, and one snapshot
// shape everything reads. The contract is fixed by tests: counters and
// bounded durations only — never plan text, evidence or secret material —
// and a nil Tracker is telemetry turned off, not an error.
package plantel

import (
	"sync"
	"time"
)

// Snapshot is the entire telemetry surface. Every field is a uint64: counters
// for events, milliseconds for durations, bytes for the prompt budget. The
// fixed, numeric-only schema is the leak contract — no string, map or slice
// field can carry plan content, tool output or unbounded labels.
type Snapshot struct {
	PlanMisses                 uint64 `json:"planMisses"`
	MaterialRevisions          uint64 `json:"materialRevisions"`
	ApprovalChurn              uint64 `json:"approvalChurn"`
	TransitionConflicts        uint64 `json:"transitionConflicts"`
	IdempotentRetries          uint64 `json:"idempotentRetries"`
	StandaloneStarts           uint64 `json:"standaloneStarts"`
	PlanOnlyRounds             uint64 `json:"planOnlyRounds"`
	ProjectionInjections       uint64 `json:"projectionInjections"`
	ProjectionBytes            uint64 `json:"projectionBytes"`
	ProjectionBytesLast        uint64 `json:"projectionBytesLast"`
	CompletionsWithoutEvidence uint64 `json:"completionsWithoutEvidence"`
	Archives                   uint64 `json:"archives"`
	ArchiveLatencyLastMS       uint64 `json:"archiveLatencyLastMs"`
	ArchiveLatencyMaxMS        uint64 `json:"archiveLatencyMaxMs"`
	DraftsAdaptive             uint64 `json:"draftsAdaptive"`
	DraftsLegacy               uint64 `json:"draftsLegacy"`
	ApprovalLatency1s          uint64 `json:"approvalLatency1s"`
	ApprovalLatency10s         uint64 `json:"approvalLatency10s"`
	ApprovalLatency60s         uint64 `json:"approvalLatency60s"`
	ApprovalLatencySlow        uint64 `json:"approvalLatencySlow"`
	MaterialReapprovals        uint64 `json:"materialReapprovals"`
	PatchRetries               uint64 `json:"patchRetries"`
	CompletionsSuccess         uint64 `json:"completionsSuccess"`
	CompletionsAbandoned       uint64 `json:"completionsAbandoned"`
}

// Tracker accumulates one plan session's telemetry. The zero Tracker counts
// nothing; a nil *Tracker records nothing and reads as zero, so callers wire
// it without an if-configured dance. It is runtime state: nothing here is
// persisted or logged, and all methods are safe for concurrent use.
type Tracker struct {
	mu       sync.Mutex
	snapshot Snapshot
}

func (t *Tracker) record(fn func(*Snapshot)) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.snapshot)
}

// Miss counts gated calls that failed plan-gate addressing: no usable step
// reference where one was required.
func (t *Tracker) Miss() {
	t.record(func(s *Snapshot) { s.PlanMisses++ })
}

// MaterialRevision counts approved-plan replacements that changed material
// fields and reset approval.
func (t *Tracker) MaterialRevision() {
	t.record(func(s *Snapshot) { s.MaterialRevisions++ })
}

// ApprovalChurn counts approval flips after the first decision: the plan
// being re-decided, not approved.
func (t *Tracker) ApprovalChurn() {
	t.record(func(s *Snapshot) { s.ApprovalChurn++ })
}

// TransitionConflict counts transitions refused by validation — the move was
// addressed at a state that cannot make it.
func (t *Tracker) TransitionConflict() {
	t.record(func(s *Snapshot) { s.TransitionConflicts++ })
}

// IdempotentRetry counts replayed mutation ids that returned a recorded
// result instead of re-applying.
func (t *Tracker) IdempotentRetry() {
	t.record(func(s *Snapshot) { s.IdempotentRetries++ })
}

// StandaloneStart counts step starts that cost their own model call because
// nothing piggybacked them.
func (t *Tracker) StandaloneStart() {
	t.record(func(s *Snapshot) { s.StandaloneStarts++ })
}

// PlanOnlyRound counts model rounds spent purely on plan calls — budget that
// carried no working tool call.
func (t *Tracker) PlanOnlyRound() {
	t.record(func(s *Snapshot) { s.PlanOnlyRounds++ })
}

// ProjectionBytes accounts the prompt budget: one call records one injection
// of n bytes, keeping the last size and the cumulative total.
func (t *Tracker) ProjectionBytes(n int) {
	t.record(func(s *Snapshot) {
		s.ProjectionInjections++
		s.ProjectionBytesLast = uint64(max(n, 0))
		s.ProjectionBytes += s.ProjectionBytesLast
	})
}

// CompletionWithoutEvidence counts completes that passed on a no-evidence
// reason instead of evidence.
func (t *Tracker) CompletionWithoutEvidence() {
	t.record(func(s *Snapshot) { s.CompletionsWithoutEvidence++ })
}

// ArchiveLatency accounts plan close latency in bounded milliseconds: one
// call is one archive, keeping the last and the running max.
func (t *Tracker) ArchiveLatency(d time.Duration) {
	// A negative duration (clock skew) clamps to zero instead of wrapping the counter.
	u := uint64(max(d.Milliseconds(), int64(0)))
	t.record(func(s *Snapshot) {
		s.Archives++
		s.ArchiveLatencyLastMS = u
		s.ArchiveLatencyMaxMS = max(s.ArchiveLatencyMaxMS, u)
	})
}

// Policy is the authoring_policy tag telemetry understands: which of the two
// fixed draft counters a draft moves. It is a number on purpose — the
// snapshot schema has no room for labels, so translating plangate's closed
// selector happens at the call site and nothing textual can enter telemetry.
type Policy uint8

const (
	// PolicyAdaptive tags a draft authored under the adaptive-minimal grammar.
	PolicyAdaptive Policy = iota
	// PolicyLegacy tags a draft authored under the legacy appendix.
	PolicyLegacy
)

// DraftCreated counts one authored plan draft, tagged by the closed
// authoring_policy selector so the two grammars stay comparable without any
// semantic scoring.
func (t *Tracker) DraftCreated(p Policy) {
	t.record(func(s *Snapshot) {
		if p == PolicyLegacy {
			s.DraftsLegacy++
			return
		}
		s.DraftsAdaptive++
	})
}

// ApprovalLatency buckets one approval decision's latency into fixed windows.
// Buckets, not timestamps or labels: the shape stays bounded whatever the
// clock does, and a negative duration (clock skew) clamps into the fastest
// bucket instead of wrapping a counter.
func (t *Tracker) ApprovalLatency(d time.Duration) {
	switch {
	case d <= time.Second:
		t.record(func(s *Snapshot) { s.ApprovalLatency1s++ })
	case d <= 10*time.Second:
		t.record(func(s *Snapshot) { s.ApprovalLatency10s++ })
	case d <= 60*time.Second:
		t.record(func(s *Snapshot) { s.ApprovalLatency60s++ })
	default:
		t.record(func(s *Snapshot) { s.ApprovalLatencySlow++ })
	}
}

// MaterialReapproval counts approvals granted to a plan after a material
// revision reset it — the re-decision itself, not the edit.
func (t *Tracker) MaterialReapproval() {
	t.record(func(s *Snapshot) { s.MaterialReapprovals++ })
}

// PatchRetry counts patch attempts refused on stale or conflicting anchors
// and retried: authoring friction the durable plan should surface.
func (t *Tracker) PatchRetry() {
	t.record(func(s *Snapshot) { s.PatchRetries++ })
}

// CompletionSuccess counts plans closed as success.
func (t *Tracker) CompletionSuccess() {
	t.record(func(s *Snapshot) { s.CompletionsSuccess++ })
}

// CompletionAbandoned counts plans closed as abandoned.
func (t *Tracker) CompletionAbandoned() {
	t.record(func(s *Snapshot) { s.CompletionsAbandoned++ })
}

// Snapshot returns the accumulated telemetry. A nil Tracker reads as zero.
func (t *Tracker) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot
}
