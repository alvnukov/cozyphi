package agent

import (
	"context"

	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planTelemetryManager returns the live session's plan telemetry sink, or nil
// when no session is wired. The sink is an atomic pointer, not a locked read:
// the projection record fires inside systemPrompt, which rebindClient runs
// under mu — the engine mutex is not reentrant. Telemetry is runtime state,
// so the live manager is the only legitimate recorder and reader.
func (engine *Engine) planTelemetryManager() *session.Manager {
	if engine == nil {
		return nil
	}
	return engine.telemetrySink.Load()
}

// recordPlanMiss counts a gated call that failed plan-gate addressing.
func (engine *Engine) recordPlanMiss() {
	if m := engine.planTelemetryManager(); m != nil {
		m.RecordPlanMiss()
	}
}

// recordPlanOnlyRound counts a model round that carried no working call.
func (engine *Engine) recordPlanOnlyRound() {
	if m := engine.planTelemetryManager(); m != nil {
		m.RecordPlanOnlyRound()
	}
}

// recordPlanStandaloneStart counts a step start the model spent a whole call
// on; the piggyback paths (auto-start, envelope settle) never reach it.
func (engine *Engine) recordPlanStandaloneStart() {
	if m := engine.planTelemetryManager(); m != nil {
		m.RecordPlanStandaloneStart()
	}
}

// recordPlanProjection accounts n bytes of plan prompt injected into the
// model context.
func (engine *Engine) recordPlanProjection(n int) {
	if m := engine.planTelemetryManager(); m != nil {
		m.RecordPlanProjection(n)
	}
}

// planTelemetry is the plan tool's diagnostics source: the bounded snapshot
// of the live session. It cannot fail — a session without telemetry reads as
// zero, never as an error.
func (engine *Engine) planTelemetry(context.Context) (plantel.Snapshot, error) {
	if m := engine.planTelemetryManager(); m != nil {
		return m.PlanTelemetry(), nil
	}
	return plantel.Snapshot{}, nil
}
