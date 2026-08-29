package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
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

func TestPlanActionCompactRunsBeforeStartTransition(t *testing.T) {
	server, _, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	var events []session.Event
	engine.sessionEvents = func(ev session.Event) { events = append(events, ev) }

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "keep context small", Approach: "compact at step start",
		SuccessCriteria: []string{"compaction ran"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "context grows here", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
			}},
		}},
	})
	seedTwoTurnHistory(t, engine)

	_, result, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "explore", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.Equal(t, session.PlanInProgress, engine.Plan().Items[0].Status)

	// The action ran before the durable write: its run record and the
	// compaction entry both exist, and the transcript heard about it.
	runs := stepRuns(t, engine, "explore")
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunOK, runs[0].Status)
	require.True(t, hasCompactionEntry(engine))
	require.Contains(t, events, session.PlanActionRan{
		StepID: "explore", Event: session.PlanActionOnStepStart,
		Type: session.PlanActionCompact, Status: session.PlanActionRunOK,
	})
}

func TestPlanActionFailureRejectsStartTransition(t *testing.T) {
	// No history seeded: compaction has nothing to compact and must fail,
	// and the failed action must refuse the transition.
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "fail loudly", Approach: "compact on an empty session",
		SuccessCriteria: []string{"transition refused"},
		Items: []session.PlanItem{{
			ID: "explore", Content: "read the code", Status: session.PlanPending, Type: session.StepExplore,
			Why: "nothing to compact yet", DoneWhen: "code is read",
			Actions: []session.PlanAction{{
				Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
			}},
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "explore", MutationID: session.NewMutationID(),
	})
	require.ErrorContains(t, err, "compact")
	require.Equal(t, session.PlanPending, engine.Plan().Items[0].Status, "the step stays where it was")
	require.False(t, hasCompactionEntry(engine))

	runs := stepRuns(t, engine, "explore")
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunFailed, runs[0].Status)
	require.NotEmpty(t, runs[0].Error)
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
	require.True(t, hasCompactionEntry(engine))
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
	require.True(t, hasCompactionEntry(engine))
}

func TestPlanStartActionFailureRejectsApproval(t *testing.T) {
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	_, _, err := engine.createPlan(t.Context(), session.PlanV2{
		Goal: "fail on approval", Approach: "nothing to compact",
		SuccessCriteria: []string{"approval refused"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
		}},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact,
		}},
	})
	require.NoError(t, err)

	_, err = engine.SetPlanApproved(true)
	require.ErrorContains(t, err, "compact")
	require.False(t, engine.Plan().Approved, "the approval write must not land")

	runs := planRuns(t, engine)
	require.Len(t, runs, 1)
	require.Equal(t, session.PlanActionRunFailed, runs[0].Status)
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
	require.True(t, hasCompactionEntry(engine))
}

func TestPlanEndActionFailureRejectsClose(t *testing.T) {
	// No history: the closing compact fails, and the whole closing transition
	// is refused — the step is not completed and the plan stays open.
	server, _, _ := fakeContextServer(t, "unused", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)

	seedApprovedActionPlan(t, engine, session.PlanV2{
		Goal: "refuse partial closure", Approach: "all or nothing",
		SuccessCriteria: []string{"close refused"},
		Items: []session.PlanItem{{
			ID: "work", Content: "do the work", Status: session.PlanPending, Type: session.StepRun,
			Why: "the plan needs a step", DoneWhen: "work is done",
		}},
		Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanEnd, Type: session.PlanActionCompact,
		}},
	})

	_, _, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionStart, StepID: "work", MutationID: session.NewMutationID(),
	})
	require.NoError(t, err)

	_, _, err = engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: session.NewMutationID(),
		Outcome: "done", Evidence: "work is done", PlanResult: session.PlanResultSuccess,
	})
	require.ErrorContains(t, err, "compact")

	plan := engine.Plan()
	require.Equal(t, session.PlanInProgress, plan.Items[0].Status, "the step stays in progress")
	require.Empty(t, plan.Result, "the plan stays open")
	require.Nil(t, plan.ClosedAt)
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
	require.False(t, hasCompactionEntry(engine))
}

func TestPlanActionsSkipReplayedMutation(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
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

	summaryRequests := len(bodies())

	// The retried transition replays the recorded result: no new run, no
	// second compaction request.
	_, result, err := engine.transitionPlan(t.Context(), session.PlanTransition{
		Action: session.TransitionComplete, StepID: "work", MutationID: mutation,
		Outcome: "done", Evidence: "work is done",
	})
	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Len(t, stepRuns(t, engine, "work"), 1)
	require.Len(t, bodies(), summaryRequests, "a replay must not re-run actions")
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
