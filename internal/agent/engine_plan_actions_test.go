package agent

import (
	"context"
	"encoding/json"
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

func TestInjectSkillStepStartQueuesInstruction(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "unused", func(int32) string { return sseTextChunk() })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "steps arrive prepared", Approach: "inject the skill at step start",
		SuccessCriteria: []string{"first turn reads the skill"},
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

	// The step's first turn carries the read-instruction; the queue drains,
	// so the next turn does not repeat it.
	drainLoop(t, engine, "continue")
	require.Contains(t, bodies()[0], "tdd", "the first turn must name the injected skill")

	drainLoop(t, engine, "again")
	// The second request still carries the first turn in history; the
	// instruction itself must appear exactly once — not re-added to the new
	// prompt.
	require.Equal(t, 1, strings.Count(bodies()[1], "You MUST read these skill files"),
		"the instruction must not repeat")
}
