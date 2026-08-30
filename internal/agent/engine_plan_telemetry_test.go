package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// newTelemetryEngine builds an engine over a persisted temp session with the
// default toolset: the plan feature is on, no model server is ever contacted —
// the tests below drive the executor and the prompt builder directly.
func newTelemetryEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(target, []byte("telemetry body"), 0o644))
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
	})
	require.NoError(t, err)
	return engine, target
}

// telemetrySnapshot reads the live session's plan telemetry. Telemetry is
// runtime state: it never persists to the session file, so the live manager
// is the only reader that sees it.
func telemetrySnapshot(t *testing.T, engine *Engine) plantel.Snapshot {
	t.Helper()
	return engine.sessionRef().manager.PlanTelemetry()
}

// createApprovedPlan drives the public plan tool: one create plus one
// approval, the way a real drafting turn does it.
func createApprovedPlan(t *testing.T, engine *Engine) {
	t.Helper()
	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
		}
	}
	require.NotNil(t, planTool, "the default toolset must carry the plan tool")
	_, err := planTool.Run(t.Context(), json.RawMessage(`{
		"action": "create",
		"goal": "two steps in two working calls",
		"approach": "auto-start then piggyback the settle",
		"successCriteria": ["no plan-only round between steps"],
		"steps": [
			{"id": "read-notes", "content": "read the notes", "status": "pending", "type": "explore", "why": "first move", "doneWhen": "content seen"},
			{"id": "read-again", "content": "read them again", "status": "pending", "type": "explore", "why": "second move", "doneWhen": "content re-seen"}
		]
	}`))
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
}

// TestEnginePlanTelemetryProjectionBytesMatchInjectedPrompt pins the byte
// accounting against an independent oracle: two otherwise identical engines,
// one with the plan feature disabled, produce system prompts whose length
// difference is exactly what the plan feature injected — and that difference,
// not a recomputed constant, is what telemetry must report.
func TestEnginePlanTelemetryProjectionBytesMatchInjectedPrompt(t *testing.T) {
	withPlan, _ := newTelemetryEngine(t)
	createApprovedPlan(t, withPlan)

	withoutPlan, _ := newTelemetryEngine(t)
	withoutPlan.SetPlanEnabled(false)

	injected := len(withPlan.systemPrompt()) - len(withoutPlan.systemPrompt())
	require.Positive(t, injected, "an approved plan must inject a non-empty prompt block")

	s := telemetrySnapshot(t, withPlan)
	assert.EqualValues(t, injected, s.ProjectionBytesLast,
		"telemetry must report exactly the bytes the plan feature injected")
	assert.GreaterOrEqual(t, s.ProjectionBytes, s.ProjectionBytesLast,
		"the cumulative total covers every injection, construction included")
	assert.Positive(t, s.ProjectionInjections)
}

// TestEnginePlanTelemetryPiggybackHappyPathZeros is the feature-value
// contract: with the settle envelope riding working calls, a whole step
// lifecycle lands with zero standalone start calls and zero plan-only model
// rounds — and none of the failure counters moves either.
func TestEnginePlanTelemetryPiggybackHappyPathZeros(t *testing.T) {
	engine, target := newTelemetryEngine(t)
	createApprovedPlan(t, engine)

	// Working round one: auto-start, no plan call.
	exec := engine.roundSnapshot().executor
	msgs, active := exec.run(t.Context(), []llm.ToolCall{
		{
			ID: "call_first",
			Function: llm.Function{
				Name:      "read",
				Arguments: `{"path":` + strconv.Quote(target) + `,"plan_step":"read-notes"}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)

	// Working round two: settle through _plan, start the next step, still no
	// plan call.
	exec = engine.roundSnapshot().executor
	msgs, active = exec.run(t.Context(), []llm.ToolCall{
		{
			ID: "call_second",
			Function: llm.Function{
				Name: "read",
				Arguments: `{"path":` + strconv.Quote(target) + `,"plan_step":"read-again",` +
					`"_plan":{"complete":{"stepId":"read-notes","outcome":"notes read","evidenceRefs":["call:call_first"]}}}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)

	// The counters only mean something if the lifecycle actually landed:
	// all-zeros over a refused settle would be the same all-zeros.
	plan := engine.sessionRef().Plan()
	statuses := map[string]session.PlanStatus{}
	for _, step := range plan.Items {
		statuses[step.ID] = step.Status
	}
	assert.Equal(t, session.PlanCompleted, statuses["read-notes"], "the settle completed step one")
	assert.Equal(t, session.PlanInProgress, statuses["read-again"], "the settle started step two")

	s := telemetrySnapshot(t, engine)
	assert.EqualValues(t, 0, s.StandaloneStarts, "auto-start must cover the starts")
	assert.EqualValues(t, 0, s.PlanOnlyRounds, "the envelope must carry the whole lifecycle")
	assert.EqualValues(t, 0, s.PlanMisses)
	assert.EqualValues(t, 0, s.TransitionConflicts)
	assert.EqualValues(t, 0, s.IdempotentRetries)
	assert.EqualValues(t, 0, s.CompletionsWithoutEvidence, "the settle carried evidence refs")
}

// TestEnginePlanTelemetryCountsGateMiss pins the miss counter on the real
// gate path: a working call that names a step the plan does not have is a
// miss, and telemetry must see it.
func TestEnginePlanTelemetryCountsGateMiss(t *testing.T) {
	engine, target := newTelemetryEngine(t)
	createApprovedPlan(t, engine)

	exec := engine.roundSnapshot().executor
	msgs, active := exec.run(t.Context(), []llm.ToolCall{
		{
			ID: "call_ghost",
			Function: llm.Function{
				Name:      "read",
				Arguments: `{"path":` + strconv.Quote(target) + `,"plan_step":"ghost-step"}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)

	s := telemetrySnapshot(t, engine)
	assert.EqualValues(t, 1, s.PlanMisses)
}

// TestEnginePlanTelemetryCountsPlanOnlyRound pins the positive path: a model
// round that spent itself on plan metadata alone — no working call anywhere —
// is exactly what the plan-only counter exists to surface.
func TestEnginePlanTelemetryCountsPlanOnlyRound(t *testing.T) {
	engine, _ := newTelemetryEngine(t)
	createApprovedPlan(t, engine)

	exec := engine.roundSnapshot().executor
	msgs, active := exec.run(t.Context(), []llm.ToolCall{{
		ID:       "call_get",
		Function: llm.Function{Name: "plan", Arguments: `{"action":"get"}`},
	}}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)

	s := telemetrySnapshot(t, engine)
	assert.EqualValues(t, 1, s.PlanOnlyRounds)
	assert.EqualValues(t, 0, s.StandaloneStarts, "reading the plan starts nothing")
	assert.EqualValues(t, 0, s.PlanMisses, "the plan tool is exempt from the gate")
}

// TestEnginePlanTelemetryCountsStandaloneStart pins the other side of the
// piggyback bargain: a start the model spent a whole plan-tool call on is a
// standalone start, even though the round is also plan-only.
func TestEnginePlanTelemetryCountsStandaloneStart(t *testing.T) {
	engine, _ := newTelemetryEngine(t)
	createApprovedPlan(t, engine)

	exec := engine.roundSnapshot().executor
	msgs, active := exec.run(t.Context(), []llm.ToolCall{
		{
			ID: "call_start",
			Function: llm.Function{
				Name:      "plan",
				Arguments: `{"action":"start","id":"read-notes","mutationId":"tel-start-1"}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)

	s := telemetrySnapshot(t, engine)
	assert.EqualValues(t, 1, s.StandaloneStarts)
	assert.EqualValues(t, 0, s.IdempotentRetries)
}

// TestEnginePlanTelemetryProjectionDedupesByteStableRerenders pins the budget
// semantics: internal rebinds (publishPlan after every durable write, memory
// sync) rebuild a byte-identical prompt, and handing the client the prompt it
// already holds spends nothing — only a changed projection counts.
func TestEnginePlanTelemetryProjectionDedupesByteStableRerenders(t *testing.T) {
	engine, _ := newTelemetryEngine(t)
	createApprovedPlan(t, engine)

	before := telemetrySnapshot(t, engine)
	require.Positive(t, before.ProjectionInjections, "drafting must have recorded projections")

	first := engine.systemPrompt()
	second := engine.systemPrompt()
	require.Equal(t, first, second, "no plan write happened between the calls")

	after := telemetrySnapshot(t, engine)
	assert.Equal(t, before.ProjectionInjections, after.ProjectionInjections,
		"a byte-stable re-render must not count as a new injection")
	assert.Equal(t, before.ProjectionBytes, after.ProjectionBytes)
}

// TestEngineCreatePlanRecordsDraftTelemetry pins the draft counter wiring:
// every authored v2 contract moves the draft counter of the grammar that
// produced it, tagged by the live policy at create time.
func TestEngineCreatePlanRecordsDraftTelemetry(t *testing.T) {
	engine, _ := newTelemetryEngine(t)

	_, _, err := engine.createPlan(t.Context(), seedContract())
	require.NoError(t, err)
	snapshot := telemetrySnapshot(t, engine)
	assert.EqualValues(t, 1, snapshot.DraftsAdaptive, "the default grammar is adaptive-minimal")
	assert.EqualValues(t, 0, snapshot.DraftsLegacy)

	defaultPolicy := plangate.DefaultDefaults()
	defaultPolicy.AuthoringPolicy = plangate.AuthoringLegacy
	require.NoError(t, engine.planRuntime.Apply(defaultPolicy))
	_, _, err = engine.createPlan(t.Context(), seedContract())
	require.NoError(t, err)
	snapshot = telemetrySnapshot(t, engine)
	assert.EqualValues(t, 1, snapshot.DraftsAdaptive)
	assert.EqualValues(t, 1, snapshot.DraftsLegacy, "the live policy tags the draft, not the compiled default")
}
