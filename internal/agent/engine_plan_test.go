package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestEnginePlanCallbackRunsAfterDurableUpdate(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	got, err := engine.updatePlan(
		t.Context(),
		[]session.PlanItem{{Content: "verify", Status: session.PlanInProgress, Type: session.StepExplore}},
	)
	require.NoError(t, err)
	assert.Equal(t, got, notified)
	assert.FileExists(t, engine.SessionFile(), "callback must not run before the plan is durable")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, got.Items, reopened.Plan().Items)
}

func TestEngineCreatesV2DraftWithoutAutoApproval(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		AutoApprove: func() bool { return true },
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	contract := session.PlanV2{
		Goal:            "ship the create/get tool actions",
		Approach:        "adapter over the canonical session model",
		SuccessCriteria: []string{"compact get stays bounded"},
		Constraints:     []string{"no schema drift"},
		Items: []session.PlanItem{{
			ID:       "wire-tool",
			Content:  "wire the tool actions",
			Status:   session.PlanPending,
			Type:     session.StepEdit,
			Why:      "the tool is the model-facing seam",
			DoneWhen: "contract tests pass",
		}},
	}
	plan, diff, err := engine.createPlan(t.Context(), contract)
	require.NoError(t, err)
	assert.True(t, plan.Schema.IsV2(), "create must store the v2 contract")
	assert.False(t, plan.Approved, "a fresh contract is a draft the user has not approved")
	assert.Equal(t, "ship the create/get tool actions", plan.Goal)
	assert.Equal(t, plan, notified)
	assert.NotEmpty(t, diff, "a first create reports the whole contract as material")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.True(t, reopened.Plan().Schema.IsV2(), "the draft must survive a reopen")

	got, err := engine.getPlan(t.Context())
	require.NoError(t, err)
	assert.Equal(t, plan.Revision, got.Revision)
	assert.Equal(t, plan.Items, got.Items)

	_, _, err = engine.createPlan(t.Context(), session.PlanV2{
		Goal:            "validate first",
		Approach:        "live policy",
		SuccessCriteria: []string{"type enforced"},
		Items: []session.PlanItem{{
			ID: "missing-type", Content: "no type", Status: session.PlanPending,
			Why: "policy check", DoneWhen: "error",
		}},
	})
	require.ErrorContains(t, err, "type is required")
}

func TestEngineWiresPlanToolCreateToDurableSession(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
	})
	require.NoError(t, err)

	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
			break
		}
	}
	require.NotNil(t, planTool, "engine must wire the plan tool")

	result, err := planTool.Run(t.Context(), json.RawMessage(`{
		"action":"create",
		"goal":"wire create through the tool",
		"approach":"engine-owned deps",
		"successCriteria":["durable draft"],
		"steps":[{"id":"wire","content":"run create through Run","status":"pending","type":"explore","why":"close the seam","doneWhen":"session holds v2"}]
	}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"action":"create"`)
	assert.True(t, engine.Plan().Schema.IsV2(), "create through the tool must reach the durable session")
	assert.False(t, engine.Plan().Approved)
	assert.Equal(t, "wire create through the tool", engine.Plan().Goal)

	// The real session's required-field text reaches the model verbatim through
	// both wraps, so the advertised contract and the durable one cannot drift.
	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action":"create",
		"approach":"missing goal",
		"successCriteria":["error text survives the wrap"],
		"steps":[{"id":"x","content":"x","status":"pending","type":"explore","why":"y","doneWhen":"z"}]
	}`))
	require.ErrorContains(t, err, "plan create: agent: create plan: session: plan goal is required")
}

func TestEngineWiresPlanToolPatchToDurableSession(t *testing.T) {
	dir := t.TempDir()
	notified := 0
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
		PlanUpdated: func(session.Plan) { notified++ },
	})
	require.NoError(t, err)

	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
			break
		}
	}
	require.NotNil(t, planTool, "engine must wire the plan tool")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "create",
		"goal": "wire patch through the engine",
		"approach": "engine-owned deps",
		"successCriteria": ["durable delta"],
		"steps": [{"id": "wire", "content": "run patch through Run", "status": "pending", "type": "explore", "why": "close the seam", "doneWhen": "session holds the delta"}]
	}`))
	require.NoError(t, err)
	createNotifications := notified

	result, err := planTool.Run(t.Context(), json.RawMessage(`{
		"action": "patch",
		"expected_revision": 1,
		"ops": [{"op": "update_step", "id": "wire", "note": "durable note"}]
	}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"action":"patch"`)
	assert.Equal(t, uint64(2), engine.Plan().Revision)
	assert.Equal(t, "durable note", engine.Plan().Items[0].Note)
	assert.Equal(t, createNotifications+1, notified, "patch publishes after the durable write")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, "durable note", reopened.Plan().Items[0].Note)

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "patch",
		"expected_revision": 1,
		"ops": [{"op": "update_step", "id": "wire", "note": "stale"}]
	}`))
	require.ErrorContains(t, err, "plan patch: agent: patch plan: session: plan revision is 2; patch expected 1")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "patch",
		"expected_revision": 2,
		"ops": [{"op": "insert_step", "after": "wire", "step": {"id": "bad", "content": "x", "type": "nope", "why": "y", "doneWhen": "z"}}]
	}`))
	require.ErrorContains(t, err, `plan patch: agent: patch plan: plangate: step 1 has unknown step type "nope"`)
}

func TestEngineWiresPlanToolTransitionToDurableSession(t *testing.T) {
	dir := t.TempDir()
	notified := 0
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
		PlanUpdated: func(session.Plan) { notified++ },
	})
	require.NoError(t, err)

	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
			break
		}
	}
	require.NotNil(t, planTool, "engine must wire the plan tool")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "create",
		"goal": "wire transitions through the engine",
		"approach": "engine-owned deps",
		"successCriteria": ["durable lifecycle"],
		"steps": [{"id": "lifecycle", "content": "run a transition through Run", "status": "pending", "type": "explore", "why": "close the seam", "doneWhen": "session holds the event"}]
	}`))
	require.NoError(t, err)
	createNotifications := notified

	result, err := planTool.Run(t.Context(), json.RawMessage(`{
		"action": "start",
		"id": "lifecycle",
		"mutationId": "wire-start-1"
	}`))
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"action":"start"`)
	assert.Equal(t, uint64(2), engine.Plan().Revision)
	assert.Equal(t, session.PlanInProgress, engine.Plan().Items[0].Status)
	assert.Equal(t, createNotifications+1, notified, "a transition publishes after the durable write")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "start",
		"id": "lifecycle",
		"mutationId": "wire-start-1"
	}`))
	require.NoError(t, err, "a retried mutation replays instead of failing")
	assert.Equal(t, uint64(2), engine.Plan().Revision, "the replay moves no revision")
	assert.Equal(t, createNotifications+1, notified, "a replay carries no new durable state and republishes nothing")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, session.PlanInProgress, reopened.Plan().Items[0].Status)
	require.Len(t, reopened.Plan().Events, 1, "the audit event is durable")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "reopen",
		"id": "lifecycle",
		"mutationId": "wire-reopen-1"
	}`))
	require.ErrorContains(
		t,
		err,
		`plan transition: agent: transition plan: session: step "lifecycle" is in_progress; allowed actions: complete, block, cancel`,
	)
}

// TestEngineAutoStartsPendingStepOnGateableCall is the tracer bullet for the
// "no separate start call" contract: one model tool call naming a pending
// step both starts the step durably and runs the tool.
func TestEngineAutoStartsPendingStepOnGateableCall(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(target, []byte("plan-gate body"), 0o644))
	notified := 0
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
		PlanUpdated: func(session.Plan) { notified++ },
	})
	require.NoError(t, err)

	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
			break
		}
	}
	require.NotNil(t, planTool)
	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "create",
		"goal": "start steps by calling tools",
		"approach": "auto-start on a gateable call",
		"successCriteria": ["one call starts and runs"],
		"steps": [{"id": "read-notes", "content": "read the notes", "status": "pending", "type": "explore", "why": "first move", "doneWhen": "content seen"}]
	}`))
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, engine.Plan().Approved)
	approvalNotifications := notified

	msgs, active, _ := engine.roundSnapshot().executor.run(t.Context(), []llm.ToolCall{
		{
			ID: "c1",
			Function: llm.Function{
				Name:      "read",
				Arguments: `{"path":` + strconv.Quote(target) + `,"plan_step":"read-notes"}`,
			},
		},
	}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "plan-gate body", "the tool ran in the same call")

	plan := engine.Plan()
	assert.Equal(t, session.PlanInProgress, plan.Items[0].Status, "the pending step started")
	assert.Equal(t, uint64(4), plan.Revision, "create, approve, auto-start, attempt: four revisions")
	require.Len(t, plan.Events, 1, "the auto-start leaves one audit event")
	assert.Equal(t, session.TransitionStart, plan.Events[0].Action)
	assert.Equal(t, "read-notes", plan.Events[0].StepID)
	assert.Equal(
		t,
		approvalNotifications+2,
		notified,
		"the start and the attempt each publish after their durable write",
	)

	require.Len(t, plan.Items[0].Attempts, 1, "the accepted call files exactly one attempt")
	attempt := plan.Items[0].Attempts[0]
	assert.Equal(t, "c1", attempt.CallID)
	assert.Equal(t, "read", attempt.Tool)
	assert.Equal(t, session.AttemptSuccess, attempt.Status)
	assert.Contains(t, attempt.Summary, "plan-gate body", "the summary is the bounded result, not raw output")

	// Evidence closes the loop: complete cites the recorded attempt, and a
	// ref naming an attempt the step never held is refused.
	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "complete",
		"id": "read-notes",
		"mutationId": "wire-complete-x",
		"outcome": "notes read",
		"evidenceRefs": ["call:missing"]
	}`))
	require.ErrorContains(t, err, `evidence ref "call:missing" is not a successful attempt of this step`)

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "complete",
		"id": "read-notes",
		"mutationId": "wire-complete-1",
		"outcome": "notes read",
		"evidenceRefs": ["call:c1"]
	}`))
	require.NoError(t, err)
	assert.Equal(t, session.PlanCompleted, engine.Plan().Items[0].Status)

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, session.PlanCompleted, reopened.Plan().Items[0].Status, "the completed step survives resume")
	require.Len(t, reopened.Plan().Items[0].Attempts, 1, "the attempt evidence survives resume")
}

// TestEnginePiggybackSettlesWithoutPlanOnlyRound is the tracer bullet for
// the api-rounds contract: two adjacent working calls carry the whole step
// lifecycle — the first auto-starts its step, the second completes it, swaps
// nothing and starts the next through the _plan envelope — and the plan tool
// is never dispatched in between. The trace between the two working rounds
// holds exactly zero plan-only model rounds.
func TestEnginePiggybackSettlesWithoutPlanOnlyRound(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(target, []byte("piggyback body"), 0o644))
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
	})
	require.NoError(t, err)

	var planTool, readTool tools.Tool
	for _, tool := range engine.buildToolList() {
		switch tool.Definition.Name {
		case "plan":
			planTool = tool
		case "read":
			readTool = tool
		}
	}
	require.NotNil(t, planTool)
	require.NotNil(t, readTool)
	_, ok := readTool.Definition.Params.Properties["_plan"]
	assert.False(t, ok, "the envelope is harness-owned: no tool schema may declare it")

	_, err = planTool.Run(t.Context(), json.RawMessage(`{
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

	// Working round one: the call names the first step; the gate auto-starts
	// it and files the attempt — no plan call needed.
	exec := engine.roundSnapshot().executor
	msgs, active, _ := exec.run(t.Context(), []llm.ToolCall{
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
	settled := engine.Plan().Revision
	require.Equal(t, session.PlanInProgress, engine.Plan().Items[0].Status)

	// Working round two: the same kind of call settles step one and starts
	// step two through _plan. Between the two rounds the model made no plan
	// tool call — the envelope carried the whole transition.
	exec = engine.roundSnapshot().executor
	msgs, active, _ = exec.run(t.Context(), []llm.ToolCall{
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
	assert.Contains(t, msgs[0].Content, "piggyback body", "the working tool ran in the settle round")

	plan := engine.Plan()
	assert.Equal(t, session.PlanCompleted, plan.Items[0].Status, "step one settled by the second working call")
	assert.Equal(t, session.PlanInProgress, plan.Items[1].Status, "step two started by the same settle")
	assert.Equal(t, settled+2, plan.Revision, "one revision for the settle, one for the new attempt")
	require.Len(t, plan.Items[0].Attempts, 1, "the first round filed its attempt evidence")
	require.Len(t, plan.Items[1].Attempts, 1, "the second round filed its attempt evidence")
	require.NotEmpty(t, plan.Mutations, "the settle lands in the mutation ledger")
	last := plan.Mutations[len(plan.Mutations)-1]
	assert.Equal(t, session.SettleAction, last.Result.Action)
	assert.Equal(t, plangate.SettleMutationID("call_second"), last.Mutation,
		"the ledger key derives from the call id, so a retry replays")

	reopened, err := session.OpenSession(engine.SessionFile())
	require.NoError(t, err)
	assert.Equal(t, session.PlanCompleted, reopened.Plan().Items[0].Status, "the settle survives resume")
	assert.Equal(t, session.PlanInProgress, reopened.Plan().Items[1].Status)
}

// Two concurrent calls naming the same pending step: one transition wins and
// the loser's failed start is re-checked against the plan and proceeds. The
// assertions hold for every interleaving, and the race detector chews on the
// real window instead of a simulation.
func TestEngineConcurrentCallsStartStepOnce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(target, []byte("race body"), 0o644))
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
	})
	require.NoError(t, err)

	var planTool tools.Tool
	for _, tool := range engine.buildToolList() {
		if tool.Definition.Name == "plan" {
			planTool = tool
			break
		}
	}
	require.NotNil(t, planTool)
	_, err = planTool.Run(t.Context(), json.RawMessage(`{
		"action": "create",
		"goal": "one start under concurrency",
		"approach": "race the calls",
		"successCriteria": ["one event"],
		"steps": [{"id": "read-notes", "content": "read the notes", "status": "pending", "type": "explore", "why": "first move", "doneWhen": "content seen"}]
	}`))
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)

	exec := engine.roundSnapshot().executor
	call := llm.ToolCall{
		ID: "c",
		Function: llm.Function{
			Name:      "read",
			Arguments: `{"path":` + strconv.Quote(target) + `,"plan_step":"read-notes"}`,
		},
	}
	contents := make([]string, 2)
	var wg sync.WaitGroup
	for i := range contents {
		wg.Go(func() {
			msgs, _, _ := exec.run(t.Context(), []llm.ToolCall{call}, func(session.ToolData) bool { return true })
			if len(msgs) == 1 {
				contents[i] = msgs[0].Content
			}
		})
	}
	wg.Wait()
	for i, content := range contents {
		assert.Contains(t, content, "race body", "call %d must run its tool whatever the interleaving", i)
	}

	plan := engine.Plan()
	assert.Equal(t, session.PlanInProgress, plan.Items[0].Status)
	starts := 0
	for _, ev := range plan.Events {
		if ev.Action == session.TransitionStart {
			starts++
		}
	}
	assert.Equal(t, 1, starts, "exactly one start transition lands, however the calls interleave")
	require.Len(t, plan.Items[0].Attempts, 1, "both calls file the same call id once")
	assert.Equal(t, session.AttemptSuccess, plan.Items[0].Attempts[0].Status)
}

func TestEngineUsesLivePolicyToValidateNewPlans(t *testing.T) {
	runtime, err := plangate.NewRuntime(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "inspect", Tools: []string{"read"},
	}}})
	require.NoError(t, err)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		PlanRuntime: runtime,
	})
	require.NoError(t, err)

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "inspect", Status: session.PlanInProgress, Type: "inspect",
	}})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "missing", Status: session.PlanInProgress,
	}})
	require.ErrorContains(t, err, "type is required")
}

func TestEngineProjectsLivePolicyOnNextInference(t *testing.T) {
	server, bodies := capturingTextServer(t)
	runtime, err := plangate.NewRuntime(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "inspect", Tools: []string{"read"},
	}}})
	require.NoError(t, err)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		PlanRuntime: runtime,
	})
	require.NoError(t, err)
	require.NoError(t, runtime.Apply(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "review", Tools: []string{"read"},
	}}}))

	drain(t, engine, "use the current policy")
	sent := bodies()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], `"review"`)
	assert.NotContains(t, sent[0], `"inspect"`)
}

func TestEngineRenamesCurrentPlanTypesWithoutDroppingApproval(t *testing.T) {
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "inspect", Status: session.PlanInProgress, Type: session.StepExplore,
	}})
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)

	plan, err := engine.RenamePlanStepTypes(t.Context(), map[session.StepType]session.StepType{
		session.StepExplore: "inspect",
	})
	require.NoError(t, err)
	assert.True(t, plan.Approved)
	assert.Equal(t, session.StepType("inspect"), plan.Items[0].Type)
	assert.Equal(t, plan, notified)
}

func TestEnginePlanCancellationDoesNotMutateOrNotify(t *testing.T) {
	notifications := 0
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		PlanUpdated: func(session.Plan) { notifications++ },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = engine.updatePlan(
		ctx,
		[]session.PlanItem{{Content: "do not store", Status: session.PlanPending, Type: session.StepExplore}},
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, notifications)
	assert.Empty(t, engine.Plan().Items)
}

// TestLoopRequestsCarryHistoryOnly: the plan is no longer injected as a
// synthetic tool round — every provider request is exactly the durable
// history, no more, no less.
func TestLoopRequestsCarryHistoryOnly(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "inspect the provider projection",
		Status:  session.PlanInProgress,
		Type:    session.StepExplore,
		Note:    "send this on every inference",
	}})
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)

	drainLoop(t, engine, "continue")
	require.Len(t, bodies(), 1)

	var request struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(bodies()[0]), &request))
	require.NotEmpty(t, request.Messages)
	assert.Equal(t, string(llm.RoleUser), request.Messages[len(request.Messages)-1].Role,
		"the request ends on the user message, not a synthetic plan round")
	for _, message := range request.Messages {
		assert.Empty(t, message.ToolCallID, "no synthetic plan snapshot joins the request")
		// The system prompt legitimately names <current-plan> in its gate prose;
		// history messages must never carry the tag itself.
		if message.Role != "system" {
			assert.NotContains(t, message.Content, "<current-plan>")
		}
	}

	for _, message := range engine.session.BuildContext() {
		assert.NotContains(t, message.Content, "<current-plan>", "provider-only context must not enter the session")
	}
}

// TestLoopRequestsStayHistoryAfterPlanUpdate: a plan tool round updates the
// durable plan mid-turn, and the next request still carries only history —
// plan state reaches the model through the system prompt, never messages.
func TestLoopRequestsStayHistoryAfterPlanUpdate(t *testing.T) {
	server, bodies := recordingServer(t, func(request int, w http.ResponseWriter) {
		if request == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_1", "plan", `{
				"steps":[{"content":"run the next round","status":"in_progress","type":"run"}]
			}`))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	drain(t, engine, "make a plan")
	sent := bodies()
	require.Len(t, sent, 2)

	for _, body := range sent {
		assert.NotContains(t, body, "<current-plan>", "plan data never rides the messages")
		assert.NotContains(t, body, "plan_snapshot", "no synthetic plan tool round is sent")
	}
}

func TestEngineApprovePlanPersistsAndNotifies(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	plan, err := engine.SetPlanApproved(true)
	require.NoError(t, err)
	assert.True(t, plan.Approved)
	assert.Equal(t, plan, notified)
	assert.Equal(t, uint64(2), plan.Revision)
}

func TestEngineApproveStepJITPersistsAndNotifies(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	_, _, err = engine.createPlan(t.Context(), session.PlanV2{
		Goal: "ship the release", Approach: "verify, publish",
		SuccessCriteria: []string{"tag is on origin"},
		Items: []session.PlanItem{{
			ID: "push-tag", Content: "push the release tag", Status: session.PlanPending,
			Type: session.StepRun, Why: "publishes", DoneWhen: "tag is on origin",
			Risk: "a published tag is irreversible", JIT: true,
		}},
	})
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)

	plan, err := engine.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)
	assert.True(t, plan.JITGranted("push-tag"), "the grant is durable")
	assert.Equal(t, plan, notified, "the grant republishes the snapshot")
	assert.True(t, plan.Approved, "the step grant never touches plan approval")
}

func TestEngineClearPlanResetsRevisionAndNotifies(t *testing.T) {
	dir := t.TempDir()
	var notified session.Plan
	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
		PlanUpdated: func(plan session.Plan) { notified = plan },
	})
	require.NoError(t, err)

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{Content: "explore", Status: session.PlanInProgress, Type: session.StepExplore},
	})
	require.NoError(t, err)

	plan, err := engine.ClearPlan()
	require.NoError(t, err)
	assert.Zero(t, plan.Revision, "clear resets the revision counter")
	assert.Empty(t, plan.Items)
	assert.Equal(t, plan, notified, "the republished empty snapshot reaches the subscriber")
}

func TestEngineGateToolListAddsPlanStepToGateableTools(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	list := engine.buildToolList()
	var read, plan, ctx *tools.Tool
	for i := range list {
		switch list[i].Definition.Name {
		case "read":
			read = &list[i]
		case "plan":
			plan = &list[i]
		case "context":
			ctx = &list[i]
		}
	}
	require.NotNil(t, read)
	require.NotNil(t, plan)
	require.NotNil(t, ctx)
	_, ok := read.Definition.Params.Properties["plan_step"]
	assert.True(t, ok)
	_, ok = plan.Definition.Params.Properties["plan_step"]
	assert.False(t, ok)
	_, ok = ctx.Definition.Params.Properties["plan_step"]
	assert.False(t, ok)
}

func TestEnginePlanGatePhaseFollowsMode(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	require.NotNil(t, engine.planGate)
	require.Equal(t, plangate.PhaseDeny, engine.planGate.Phase, "useplan defaults to deny")

	engine.SetMode(ModeUsePlan)
	require.Equal(t, plangate.PhaseDeny, engine.planGate.Phase, "useplan must deny misses")

	engine.SetMode(ModePlan)
	require.Equal(t, plangate.PhaseHint, engine.planGate.Phase, "plan stays hint")

	engine.SetMode(ModeBuild)
	require.Equal(t, plangate.PhaseHint, engine.planGate.Phase, "build stays hint")
}

func TestEngineToolListInjectsPlanStepIntoMetaTools(t *testing.T) {
	pool := mcp.NewPool(map[string]mcp.ServerConfig{"echo": {Command: []string{"true"}}})
	mgr, err := job.New(job.Options{
		Root:   t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) { return "ok", nil }),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		MCP:         pool,
		Jobs:        mgr,
	})
	require.NoError(t, err)

	list := engine.buildToolList()
	for _, name := range []string{"agent_spawn", "agent_wait", "mcp_list", "mcp_call"} {
		var tool *tools.Tool
		for i := range list {
			if list[i].Definition.Name == name {
				tool = &list[i]
				break
			}
		}
		require.NotNil(t, tool, name)
		_, ok := tool.Definition.Params.Properties["plan_step"]
		assert.True(t, ok, "%s must gain plan_step", name)
	}
}

func TestEnginePromptUsePlanBlocksTool(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	engine.SetMode(ModeUsePlan)
	assert.Contains(t, engine.systemPrompt(), "blocks the tool")

	engine.SetMode(ModeBuild)
	assert.NotContains(t, engine.systemPrompt(), "blocks the tool", "build only hints misses")
}

func TestEnginePromptCarriesPlanGateBlock(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	prompt := engine.systemPrompt()
	assert.Contains(t, prompt, "Plan gate")
	assert.Contains(t, prompt, "plan_step")
	assert.Contains(t, prompt, "explore")
}

// TestEnginePromptStaysByteStableAcrossPlanWrites pins the provider prefix
// cache contract: the plan hint rides the tail of the system prompt, so a
// per-write change there orphaned the whole conversation history from the
// cache and re-billed it on every plan mutation. Plan writes (new revision,
// step transitions, approval flips) must leave the prompt byte-identical;
// volatile state reaches the model through persisted tool results.
func TestEnginePromptStaysByteStableAcrossPlanWrites(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{ID: "s1", Content: "first shape", Status: session.PlanInProgress, Type: session.StepEdit},
	})
	require.NoError(t, err)
	before := engine.systemPrompt()
	assert.Contains(t, before, "durable plan", "a live plan is announced")

	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{
		{ID: "s1", Content: "rewritten shape", Status: session.PlanCompleted, Type: session.StepEdit},
		{ID: "s2", Content: "a second step", Status: session.PlanPending, Type: session.StepRun},
	})
	require.NoError(t, err)

	assert.Equal(t, before, engine.systemPrompt(),
		"plan writes must not change the system-prompt bytes the provider caches")
}

func TestSetPlanEnabledTogglesFeatureLive(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content: "keep me", Status: session.PlanPending, Type: session.StepRun,
	}})
	require.NoError(t, err)
	require.True(t, engine.HasTool("plan"))
	require.Contains(t, engine.ToolNames(), "plan")
	require.Contains(t, engine.systemPrompt(), "Plan gate")

	engine.SetPlanEnabled(false)
	assert.False(t, engine.HasTool("plan"), "the plan tool leaves the executor registry")
	assert.NotContains(t, engine.ToolNames(), "plan", "the provider never sees the plan tool")
	assert.NotContains(t, engine.systemPrompt(), "Plan gate", "no gate block without the feature")
	assert.NotContains(t, engine.systemPrompt(), "plan_step", "no plan hint without the feature")

	engine.SetPlanEnabled(true)
	assert.True(t, engine.HasTool("plan"))
	assert.Contains(t, engine.ToolNames(), "plan")
	assert.Contains(t, engine.systemPrompt(), "Plan gate")
	require.NotEmpty(t, engine.Plan().Items)
	assert.Equal(t, "keep me", engine.Plan().Items[0].Content, "the durable plan survives the switch")
}

func TestSetPlanEnabledStepsDownFromPlanMode(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	engine.SetMode(ModePlan)
	require.Equal(t, ModePlan, engine.Mode())

	engine.SetPlanEnabled(false)
	assert.Equal(t, ModeUsePlan, engine.Mode(), "plan mode steps down when the feature turns off")

	engine.SetPlanEnabled(true)
	assert.Equal(t, ModeUsePlan, engine.Mode(), "re-enabling restores the feature, not the stale mode")
}

func TestSetPlanEnabledIgnoresEnginesWithoutPlanRuntime(t *testing.T) {
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir(), ParentID: "job_1"},
	})
	require.NoError(t, err)
	require.False(t, engine.HasTool("plan"), "sub-agents start plan-less")

	engine.SetPlanEnabled(true)
	assert.False(t, engine.HasTool("plan"), "no runtime to rebind against: the call is a no-op")
}

func TestPlanToolAutoApprovalIsTruthfulOnWire(t *testing.T) {
	server, bodies := recordingServer(t, func(request int, w http.ResponseWriter) {
		switch request {
		case 1:
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_active", "plan", `{
				"steps":[{"content":"ship it","status":"in_progress","type":"edit"}]
			}`))
		case 2:
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_closed", "plan", `{
				"steps":[{"content":"ship it","status":"completed","type":"edit"}]
			}`))
		default:
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
	})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return true },
	})
	require.NoError(t, err)

	drain(t, engine, "make and complete a plan")
	sent := bodies()
	require.Len(t, sent, 3)
	assert.Contains(t, toolResultContent(t, sent[1], "call_active"), `"approved":true`)
	assert.Contains(t, toolResultContent(t, sent[2], "call_closed"), `"approved":false`)
	assert.False(t, engine.Plan().Approved, "a completed plan must be durably unapproved before the tool returns")
}

func TestPlanToolLeavesApprovalOffOnWire(t *testing.T) {
	server, bodies := recordingServer(t, func(request int, w http.ResponseWriter) {
		if request == 1 {
			_, _ = fmt.Fprint(w, sseToolCallChunk("call_plan", "plan", `{
				"steps":[{"content":"inspect","status":"in_progress","type":"explore"}]
			}`))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		AutoApprove: func() bool { return false },
	})
	require.NoError(t, err)

	drain(t, engine, "make a plan")
	sent := bodies()
	require.Len(t, sent, 2)
	assert.Contains(t, toolResultContent(t, sent[1], "call_plan"), `"approved":false`)
}

func toolResultContent(t *testing.T, body, callID string) string {
	t.Helper()
	var request struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	for _, message := range request.Messages {
		if message.Role == string(llm.RoleTool) && message.ToolCallID == callID {
			return message.Content
		}
	}
	require.FailNow(t, "tool result not found", "tool_call_id=%s", callID)
	return ""
}
