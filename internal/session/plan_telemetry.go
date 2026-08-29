package session

import "github.com/alvnukov/cozyphi/internal/plantel"

// PlanTelemetry returns the bounded plan observability snapshot: counters
// that moved on public plan operations. Telemetry is runtime state — it is
// never persisted, and it counts operations, never plan content. A Manager
// without a tracker (direct literals in tests) reads as zero.
func (sm *Manager) PlanTelemetry() plantel.Snapshot {
	if sm == nil {
		return plantel.Snapshot{}
	}
	return sm.telemetry.Snapshot()
}

// RecordPlanMiss counts a gated call that failed plan-gate addressing: no
// usable step reference where the gate required one. The engine records; the
// session owns the counter.
func (sm *Manager) RecordPlanMiss() {
	if sm == nil {
		return
	}
	sm.telemetry.Miss()
}

// RecordPlanStandaloneStart counts a step start that cost its own model call
// because no working call piggybacked it.
func (sm *Manager) RecordPlanStandaloneStart() {
	if sm == nil {
		return
	}
	sm.telemetry.StandaloneStart()
}

// RecordPlanOnlyRound counts a model round spent purely on plan calls —
// budget that carried no working tool call.
func (sm *Manager) RecordPlanOnlyRound() {
	if sm == nil {
		return
	}
	sm.telemetry.PlanOnlyRound()
}

// RecordPlanProjection accounts n bytes of plan prompt injected into the
// model context.
func (sm *Manager) RecordPlanProjection(n int) {
	if sm == nil {
		return
	}
	sm.telemetry.ProjectionBytes(n)
}
