package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// seedApprovedActionPlan stores a v2 contract and approves it, so transitions
// on it are automation-eligible. Returns nothing: the engine snapshot is the
// source of truth for the assertions.
func seedApprovedActionPlan(t *testing.T, engine *Engine, contract session.PlanV2) {
	t.Helper()
	_, _, err := engine.createPlan(t.Context(), contract)
	require.NoError(t, err)
	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
}

// hasCompactionEntry reports whether the durable session log already carries
// a compaction record: the observable proof that a compact action ran.
func hasCompactionEntry(engine *Engine) bool {
	return slices.ContainsFunc(engine.session.PathEntries(), func(e session.MessageEntry) bool {
		_, ok := e.(session.CompactionEntry)
		return ok
	})
}

// stepRuns reads the recorded run tail of a step's action list.
func stepRuns(t *testing.T, engine *Engine, stepID string) []session.PlanActionRun {
	t.Helper()
	plan := engine.Plan()
	idx := slices.IndexFunc(plan.Items, func(item session.PlanItem) bool { return item.ID == stepID })
	require.GreaterOrEqual(t, idx, 0, "step must exist in the plan")
	require.NotEmpty(t, plan.Items[idx].Actions, "action must exist on the step")
	return plan.Items[idx].Actions[0].Runs
}

func installPlanSkill(t *testing.T, engine *Engine, name, body string) {
	t.Helper()
	if engine.skillPath == "" {
		engine.skillPath = t.TempDir()
	}
	dir := filepath.Join(engine.skillPath, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: test skill\n---\n"+body),
		0o644,
	))
}

func requestMessageText(t *testing.T, raw string, latestUserOnly bool) string {
	t.Helper()
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	if latestUserOnly {
		for i, msg := range slices.Backward(request.Messages) {
			_ = i
			if msg.Role == "user" {
				return msg.Content
			}
		}
		return ""
	}
	var content strings.Builder
	for _, message := range request.Messages {
		content.WriteString(message.Content)
		content.WriteByte('\n')
	}
	return content.String()
}

// planRuns reads the recorded run tail of the plan-level action list.
func planRuns(t *testing.T, engine *Engine) []session.PlanActionRun {
	t.Helper()
	plan := engine.Plan()
	require.NotEmpty(t, plan.Actions, "action must exist on the plan")
	return plan.Actions[0].Runs
}

func TestPlanActionCompactQueuesAdviceForNextPrompt(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)

	var events []session.Event
	engine.sessionEvents = func(ev session.Event) { events = append(events, ev) }

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "keep context small", Approach: "advise compaction at step start",
		SuccessCriteria: []string{"the model is told to compact"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "context grows here", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
			}},
		}},
	})

	_, result, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "explore", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.Equal(t, session.PlanInProgress, engine.Plan().Items[0].Status)

	// The action ran before the durable write: its run record exists and the
	// transcript heard about it — but no compaction ran, only the advice.
	runs := stepRuns(t, engine, "explore")
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunOK, runs[0].Status)
	require.False(t, hasCompactionEntry(engine), "a compact action must not compact on its own")
	require.Contains(t, events, session.PlanActionRan{
		StepID: "explore", Event: session.PlanActionOnStepStart,
		Type: session.PlanActionCompact, Status: session.PlanActionRunOK,
	})

	// The step's first turn carries the advice; the queue drains, so the
	// next turn does not repeat it.
	drainLoop(t, engine, "continue")
	require.Contains(t, bodies()[0], "recommends compacting the context now")

	drainLoop(t, engine, "again")
	require.Equal(t, 1, strings.Count(bodies()[1], "recommends compacting"),
		"the advice must ride exactly one prompt")
}

func TestPlanStartActionFiresOnAutoApproval(t *testing.T) {
	server, _, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.autoApprove = func() bool { return true }

	_, _, err := engine.createPlan(t.Context(), session.PlanV2{
		Goal: "compact on auto approval", Approach: "the policy door shares the approval batch",
		SuccessCriteria: []string{"compaction ran when the policy approved"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
		}},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
		}},
	})
	require.NoError(t, err)
	require.False(t, engine.Plan().Approved, "a fresh draft is unapproved")
	seedTwoTurnHistory(t, engine)

	// Any patch under the auto-approval policy approves the plan; the
	// plan_start batch must fire exactly like the TUI approval door.
	plan, _, err := engine.PatchPlan(t.Context(), engine.Plan().Revision, []session.PlanPatchOp{{
		Op:             session.PlanPatchReplaceContext,
		WorkingContext: session.PatchValue[string]{Set: true, Value: "fired"},
	}})
	require.NoError(t, err)
	require.True(t, plan.Approved, "the policy approves the revision")

	runs := planRuns(t, engine)
	require.Len(t, runs, 1, "the plan_start batch ran with the auto-approval")
	require.Equal(t, session.PlanActionRunOK, runs[0].Status)
	require.NotEmpty(t, engine.compactAdvice, "the action queued the compaction advice")
}

// TestPlanActionAdviceRidesItsOwnToolResult pins the in-call delivery: a
// compaction recommendation parked by a tool's own Run (a plan compact
// action in the call's settle, or in the plan tool's transition) reaches the
// model as part of that call's tool result, not one prompt later.
func TestPlanActionAdviceRidesItsOwnToolResult(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "unused", func(n int32) string {
		if n == 1 {
			return sseToolCallChunk("call_1", "trip", `{}`)
		}
		return sseTextChunk()
	})

	var engine *Engine
	trip := tools.Tool{
		Definition: llm.ToolDefinition{Name: "trip"},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			engine.queuePlanCompactAdvice()
			return tools.Result{Content: "tripped"}, nil
		},
	}
	var err error
	engine, err = NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x", ContextWindow: 100000},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{trip},
	})
	require.NoError(t, err)

	// drainLoop stops at the first complete assistant update — a tool-call
	// round included — which would cancel the very round under test; consume
	// the full turn instead.
	var tripDone bool
	var loopErr error
	for ev, err := range engine.Loop(t.Context(), "go", LoopOpts{}) {
		if err != nil {
			loopErr = err
			break
		}
		if td, ok := ev.(session.ToolData); ok && td.Run.Name == "trip" && td.Run.Status == session.ToolDone {
			tripDone = true
		}
	}
	require.NoError(t, loopErr)
	require.True(t, tripDone, "the trip call must execute and complete")
	require.Equal(t, int32(2), streams.Load())

	// Round two's request carries the advice inside trip's own tool result
	// — the boundary the action fired at — exactly once.
	snapshot := bodies()
	require.Len(t, snapshot, 2)
	require.Equal(t, 1, strings.Count(snapshot[1], "recommends compacting"),
		"the advice must ride the tool result of the call that parked it")

	// The queue drained in-call, so the next prompt prepends nothing; the
	// history copy from trip's result is the only one left.
	for ev, err := range engine.Loop(t.Context(), "again", LoopOpts{}) {
		if err != nil {
			t.Fatalf("second loop: %v", err)
		}
		_ = ev
	}
	final := bodies()
	require.NotEmpty(t, final)
	require.Equal(t, 1, strings.Count(final[len(final)-1], "recommends compacting"),
		"the next prompt must not prepend a second copy")
}

// TestPlanSkillPreloadsBeforeFirstWorkingDispatch pins the step-start boundary:
// the call that starts a skill-bearing step is refused after the start action
// lands but before its tool runs. Its result carries the complete skill body,
// so retrying the same working call runs under that guidance.
func TestPlanSkillPreloadsBeforeFirstWorkingDispatch(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	const body = "PRELOAD-SENTINEL first rule\n\nPRELOAD-SENTINEL second rule"
	installPlanSkill(t, engine, "tdd", body)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "prepare before work", Approach: "preload the step skill",
		SuccessCriteria: []string{"no work runs before guidance"},
		Items: []session.PlanItem{{
			ID: "work", Content: "run the tool", Status: session.PlanPending, Type: session.StepRun,
			Why: "the first call needs guidance", DoneWhen: "the tool ran",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill, Skills: []string{"tdd"},
			}},
		}},
	})

	runs := 0
	engine.executor.registry["bash"] = tools.Tool{
		Definition: llm.ToolDefinition{Name: "bash"},
		Run: func(context.Context, json.RawMessage) (tools.Result, error) {
			runs++
			return tools.Result{Content: "tripped"}, nil
		},
	}
	call := llm.ToolCall{
		ID: "call_1", Function: llm.Function{Name: "bash", Arguments: `{"command":"true","plan_step":"work"}`},
	}

	parallelCall := call
	parallelCall.ID = "call_2"
	first, active := engine.executor.run(
		t.Context(), []llm.ToolCall{call, parallelCall}, func(session.ToolData) bool { return true },
	)
	require.True(t, active)
	require.Len(t, first, 2)
	require.Zero(t, runs, "no call in the starting batch may dispatch before the model receives the skill")
	require.Equal(t, session.PlanInProgress, engine.Plan().Items[0].Status, "the refused call still starts the step")
	require.Len(t, stepRuns(t, engine, "work"), 1, "the inject action keeps its durable run")
	require.Equal(t, session.PlanActionRunOK, stepRuns(t, engine, "work")[0].Status)
	require.Contains(t, first[0].Content, body, "the harness returns the complete plain-text skill body")
	require.Contains(t, first[0].Content, "retry")
	require.NotContains(t, first[0].Content, "You MUST read these skill files")
	require.NotContains(t, first[0].Content, "@file ", "skill preload is not hashline tool output")
	require.Contains(t, first[1].Content, "not executed", "later calls in the same batch are also refused")

	call.ID = "call_3"
	second, active := engine.executor.run(
		t.Context(),
		[]llm.ToolCall{call},
		func(session.ToolData) bool { return true },
	)
	require.True(t, active)
	require.Len(t, second, 1)
	require.Equal(t, 1, runs, "the retry is the first working dispatch")
	require.Equal(t, "tripped", second[0].Content)
	require.NotContains(t, second[0].Content, body, "the drained skill is not injected twice")
}

func TestPlanStartActionFiresOnApproval(t *testing.T) {
	server, _, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	_, _, err := engine.createPlan(t.Context(), session.PlanV2{
		Goal: "compact on approval", Approach: "plan-level housework",
		SuccessCriteria: []string{"compaction ran at approval"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
		}},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
		}},
	})
	require.NoError(t, err)
	seedTwoTurnHistory(t, engine)

	_, err = engine.SetPlanApproved(true)
	require.NoError(t, err)
	require.True(t, engine.Plan().Approved)

	runs := planRuns(t, engine)
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunOK, runs[0].Status)
	require.NotEmpty(t, engine.compactAdvice, "the action queued the compaction advice")
}

func TestPlanEndActionRunsOnClosingComplete(t *testing.T) {
	server, _, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "compact on close", Approach: "leave a small context behind",
		SuccessCriteria: []string{"compaction ran at close"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
		}},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanEnd, Type: session.PlanActionCompact,
		}},
	})
	seedTwoTurnHistory(t, engine)

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	plan, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "done", Evidence: "work is done", PlanResult: session.PlanResultSuccess,
	})
	require.NoError(t, err)
	require.Equal(t, session.PlanResultSuccess, plan.Result)
	require.NotNil(t, plan.ClosedAt)

	runs := planRuns(t, engine)
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunOK, runs[0].Status)
	require.NotEmpty(t, engine.compactAdvice, "the action queued the compaction advice")
}

// TestPlanCompleteFromPendingRefusesUnfiredStepStartAutomation: a step that
// never started still owes its start automation. Completing it from pending
// would record the work as done while the plan's step_start contract silently
// never ran — the transition refuses instead, and the step stays pending.
func TestPlanCompleteFromPendingRefusesUnfiredStepStartAutomation(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "no silent skips", Approach: "completion only through a start",
		SuccessCriteria: []string{"pending completion with start automation refuses"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "the automation must fire", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill, Skills: []string{"tdd"},
			}},
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "explore", MutationID: session.NewMutationID(),
		Outcome: "read the changelog", Evidence: "quoted it in chat",
	})
	require.ErrorContains(t, err, "step_start")
	require.ErrorContains(t, err, "explore")
	require.Equal(t, session.PlanPending, engine.Plan().Items[0].Status, "the step stays pending")
	require.Empty(t, stepRuns(t, engine, "explore"), "no action ran on the refused path")
}

// TestPlanCompleteFromPendingRefusesUnstartedModelPin: the model pin is part
// of the step's start contract too — a complete that never switched to it is
// a skipped promise, refused loudly rather than recorded as done.
func TestPlanCompleteFromPendingRefusesUnstartedModelPin(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	engine.resolveModel = resolveOnly(server.URL)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "the pin is a promise", Approach: "switch before completion",
		SuccessCriteria: []string{"pending completion with a model pin refuses"},
		Items: []session.PlanItem{{
			ID: "work", Content: "change the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the pin must apply before done", DoneWhen: "code is changed", Model: "plan-b",
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "edited", Evidence: "diff in chat",
	})
	require.ErrorContains(t, err, "model")
	require.Equal(t, session.PlanPending, engine.Plan().Items[0].Status)
}

// TestPlanCompleteFromPendingPlainStepStillPasses: steps with no start
// automation keep the one-call door — completing plain pending work stays
// legal, so the refusal narrows to the contract the step actually promised.
func TestPlanCompleteFromPendingPlainStepStillPasses(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "tiny steps stay tiny", Approach: "no automation, no ceremony",
		SuccessCriteria: []string{"plain pending completion succeeds"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "nothing was promised at start", DoneWhen: "work is done",
		}},
	})

	plan, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "done", Evidence: "work is done",
	})
	require.NoError(t, err)
	require.Equal(t, session.PlanCompleted, plan.Items[0].Status)
}

// TestSettleCompleteFromPendingRefusesUnfiredStepStartAutomation: the _plan
// envelope is the second completion door — it owes the same refusal when the
// step it settles never started.
func TestSettleCompleteFromPendingRefusesUnfiredStepStartAutomation(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "the envelope too", Approach: "refuse the silent skip on settle",
		SuccessCriteria: []string{"settle completion of an unstarted step refuses"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "start before settle-complete", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill, Skills: []string{"tdd"},
			}},
		}},
	})

	err := engine.settlePlanFromCall(t.Context(), session.PlanSettle{
		MutationID: session.NewMutationID(),
		Complete: &session.PlanTransition{
			Action: session.TransitionComplete, StepID: "explore", Outcome: "read", Evidence: "quoted in chat",
		},
	})
	require.ErrorContains(t, err, "step_start")
	require.Equal(t, session.PlanPending, engine.Plan().Items[0].Status, "the settle must not land")
}

func TestPlanActionsSkipUnapprovedDrafts(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	_, _, err := engine.createPlan(t.Context(), session.PlanV2{
		Goal: "drafts stay passive", Approach: "automation only after approval",
		SuccessCriteria: []string{"no automation in drafts"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "drafts do not run actions", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
			}},
		}},
	})
	require.NoError(t, err)
	seedTwoTurnHistory(t, engine)

	// Whether the session allows transitions on a draft is its call; the
	// invariant under test is that no automation fired either way.
	_, _, _ = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "explore", MutationID: session.NewMutationID(),
	})

	require.Empty(t, stepRuns(t, engine, "explore"), "a draft must not run actions")
	require.Empty(t, engine.compactAdvice, "a draft must not queue advice")
}

func TestPlanActionsSkipReplayedMutation(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "replays are idempotent", Approach: "actions fire once per mutation",
		SuccessCriteria: []string{"no double compaction"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepEnd, Type: session.PlanActionCompact,
			}},
		}},
	})
	seedTwoTurnHistory(t, engine)

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	mutation := session.NewMutationID()
	_, _, err = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: mutation,
		Outcome: "done", Evidence: "work is done",
	})
	require.NoError(t, err)
	require.Len(t, stepRuns(t, engine, "work"), 1)

	// The completed step queued the advice; consume it as the prompt would.
	require.NotEmpty(t, engine.compactAdvice, "the step_end action queued advice")
	engine.compactAdvice = ""

	// The retried transition replays the recorded result: no new run, and
	// the advice is not queued a second time.
	_, result, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: mutation,
		Outcome: "done", Evidence: "work is done",
	})
	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Len(t, stepRuns(t, engine, "work"), 1)
	require.Empty(t, engine.compactAdvice, "a replay must not re-queue the advice")
}

func TestInjectSkillStepStartPreloadsBodyIntoNextPrompt(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	const body = "STARTED-BEFORE-LOOP first instruction\n\nSTARTED-BEFORE-LOOP final instruction"
	installPlanSkill(t, engine, "tdd", body)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "steps arrive prepared", Approach: "inject the skill at step start",
		SuccessCriteria: []string{"first turn has the complete skill"},
		Items: []session.PlanItem{{
			ID: "edit", Content: "edit the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the step needs its skill", DoneWhen: "code is edited",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
				Skills: []string{"tdd"},
			}},
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "edit", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	require.Len(t, stepRuns(t, engine, "edit"), 1)
	require.Equal(t, session.PlanActionRunOK, stepRuns(t, engine, "edit")[0].Status)

	// A step started before Loop parks its skill until prompt composition.
	drainLoop(t, engine, "continue")
	firstPrompt := requestMessageText(t, bodies()[0], true)
	require.Contains(t, firstPrompt, body)
	require.NotContains(t, firstPrompt, "You MUST read these skill files")
	require.NotContains(t, firstPrompt, "@file ")

	drainLoop(t, engine, "again")
	allMessages := requestMessageText(t, bodies()[1], false)
	require.Equal(t, 1, strings.Count(allMessages, body), "the complete body must not repeat")
}

// TestInjectSkillQueuesOnlyEffectiveSkills: the user's off marks ride the
// action; injection must honor them, and an action with nothing left to
// inject is a quiet no-op whose run still records OK.
func TestInjectSkillQueuesOnlyEffectiveSkills(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	installPlanSkill(t, engine, "tdd", "DISABLED-TDD-BODY")
	installPlanSkill(t, engine, "grill", "ENABLED-GRILL-BODY")
	installPlanSkill(t, engine, "implement", "ENABLED-IMPLEMENT-BODY")
	installPlanSkill(t, engine, "code-review", "DISABLED-REVIEW-BODY")

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "off skills stay out", Approach: "inject only what is on",
		SuccessCriteria: []string{"disabled names never reach the prompt"},
		Items: []session.PlanItem{{
			ID: "edit", Content: "edit the code", Status: session.PlanPending, Type: session.StepEdit,
			Why: "the step has skills", DoneWhen: "code is edited",
			Actions: []session.PlanAction{
				{
					Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
					Skills: []string{"tdd", "grill", "implement"}, DisabledSkills: []string{"tdd"},
				},
				{
					Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
					Skills: []string{"grill"},
				},
				{
					Event: session.PlanActionOnStepStart, Type: session.PlanActionInjectSkill,
					Skills: []string{"code-review"}, DisabledSkills: []string{"code-review"},
				},
			},
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "edit", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	actions := engine.Plan().Items[slices.IndexFunc(engine.Plan().Items, func(it session.PlanItem) bool {
		return it.ID == "edit"
	})].Actions
	require.Len(t, actions, 3, "all authored actions survive the plan write")
	for i, action := range actions {
		require.Len(t, action.Runs, 1, "action %d runs even when fully off", i)
		require.Equal(t, session.PlanActionRunOK, action.Runs[0].Status)
	}

	drainLoop(t, engine, "continue")
	prompt := requestMessageText(t, bodies()[0], true)
	require.Contains(t, prompt, "ENABLED-GRILL-BODY", "the enabled skill body is injected")
	require.Contains(t, prompt, "ENABLED-IMPLEMENT-BODY")
	require.Less(t, strings.Index(prompt, "ENABLED-GRILL-BODY"), strings.Index(prompt, "ENABLED-IMPLEMENT-BODY"),
		"skill action order is preserved")
	require.Equal(t, 1, strings.Count(prompt, "ENABLED-GRILL-BODY"), "duplicate skill names load once")
	require.NotContains(t, prompt, "DISABLED-TDD-BODY", "the off skill must not reach the prompt")
	require.NotContains(t, prompt, "DISABLED-REVIEW-BODY", "a fully disabled action injects nothing")
	require.NotContains(t, prompt, "You MUST read these skill files")
}

// A plan can name a skill the catalog cannot supply — a typo, or a skill that
// was removed since the plan was authored. The step must still be told to get
// that guidance instead of being handed a heading with nothing under it.
func TestPlanSkillPreloadFallsBackWhenBodyIsMissing(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)
	installPlanSkill(t, engine, "tdd", "Write the failing test first.")

	engine.queuePlanSkills([]string{"tdd", "no-such-skill"})
	preload := engine.drainPlanSkills()

	require.Contains(t, preload, "## Skill: tdd")
	require.Contains(t, preload, "Write the failing test first.")
	require.NotContains(t, preload, "## Skill: no-such-skill",
		"a skill with no body must not be announced as preloaded")
	require.Contains(t, preload, "You MUST read these skill files first")
	require.Contains(t, preload, "no-such-skill")
}

func TestPlanSkillPreloadWithoutAnyBodyIsOnlyAReadInstruction(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)

	engine.queuePlanSkills([]string{"gone"})
	preload := engine.drainPlanSkills()

	require.NotContains(t, preload, "preloaded")
	require.Contains(t, preload, "You MUST read these skill files first")
	require.Contains(t, preload, "gone")
}
