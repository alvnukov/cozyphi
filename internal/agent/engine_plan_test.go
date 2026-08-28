package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

	msgs, active := engine.roundSnapshot().executor.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "read", Arguments: `{"path":"` + target + `","plan_step":"read-notes"}`},
	}}, func(session.ToolData) bool { return true })
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
		ID:       "c",
		Function: llm.Function{Name: "read", Arguments: `{"path":"` + target + `","plan_step":"read-notes"}`},
	}
	contents := make([]string, 2)
	var wg sync.WaitGroup
	for i := range contents {
		wg.Go(func() {
			msgs, _ := exec.run(t.Context(), []llm.ToolCall{call}, func(session.ToolData) bool { return true })
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

func TestLoopInjectsCurrentPlanWithoutPersistingSnapshot(t *testing.T) {
	server, bodies := capturingTextServer(t)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)
	_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
		Content:  "inspect the provider projection",
		Status:   session.PlanInProgress,
		Type:     session.StepExplore,
		Note:     "send this on every inference",
		Evidence: "never persist the synthetic message",
	}})
	require.NoError(t, err)
	plan, err := engine.SetPlanApproved(true)
	require.NoError(t, err)

	drainLoop(t, engine, "continue")
	require.Len(t, bodies(), 1)

	var request struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(bodies()[0]), &request))
	require.NotEmpty(t, request.Messages)
	caller := request.Messages[len(request.Messages)-2]
	assert.Equal(t, string(llm.RoleAssistant), caller.Role, "the synthetic plan snapshot is presented as a tool round")
	require.Len(t, caller.ToolCalls, 1)
	assert.Equal(t, "plan", caller.ToolCalls[0].Function.Name)
	injected := request.Messages[len(request.Messages)-1]
	assert.Equal(t, string(llm.RoleTool), injected.Role)
	assert.Equal(t, caller.ToolCalls[0].ID, injected.ToolCallID)
	assert.Contains(t, injected.Content, "<current-plan>")
	assert.Contains(t, injected.Content, `"revision":`+fmt.Sprint(plan.Revision))
	assert.Contains(t, injected.Content, `"approved":true`)
	assert.Contains(t, injected.Content, "inspect the provider projection")
	assert.Contains(t, injected.Content, "send this on every inference")
	assert.Contains(t, injected.Content, `"status":"in_progress"`)
	assert.NotContains(t, injected.Content, "never persist the synthetic message",
		"evidence prose is audit and stays out of the injected snapshot")

	for _, message := range engine.session.BuildContext() {
		assert.NotContains(t, message.Content, "<current-plan>", "provider-only context must not enter the session")
	}
}

func TestLoopRefreshesPlanSnapshotAfterUpdateToolRound(t *testing.T) {
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

	contents := make([]string, 0, len(sent))
	for _, body := range sent {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		require.NotEmpty(t, request.Messages)
		contents = append(contents, request.Messages[len(request.Messages)-1].Content)
	}
	assert.Contains(t, contents[0], `"revision":0`)
	assert.Contains(t, contents[0], `"approved":false`)
	assert.NotContains(t, contents[0], "run the next round")
	assert.Contains(t, contents[1], `"revision":1`)
	assert.Contains(t, contents[1], "run the next round")
	assert.Contains(t, contents[1], `"status":"in_progress"`)
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
