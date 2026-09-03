package session

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transitionFixture returns a manager holding a two-step approved v2 plan at
// revision 2: step "alpha" sits in the requested status, step "beta" is
// completed, so the single-in-progress invariant never interferes with the
// step under test.
func transitionFixture(t *testing.T, status PlanStatus) *Manager {
	t.Helper()
	contract := v2Fixture()
	contract.Items = []PlanItem{
		{
			ID:       "alpha",
			Content:  "drive the step lifecycle",
			Status:   status,
			Type:     StepEdit,
			Why:      "transitions need a target",
			DoneWhen: "status machine works",
		},
		{
			ID:       "beta",
			Content:  "already finished neighbor",
			Status:   PlanCompleted,
			Type:     StepExplore,
			Why:      "keeps the plan realistic",
			DoneWhen: "landed",
			Outcome:  "done earlier",
			Evidence: "prior run",
		},
	}
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	require.Equal(t, uint64(2), m.Plan().Revision)
	require.True(t, m.Plan().Approved)
	return m
}

// transitionPayload builds one valid transition of each action for step alpha.
func transitionPayload(action, mutation string) PlanTransition {
	tr := PlanTransition{Action: action, StepID: "alpha", MutationID: mutation}
	switch action {
	case TransitionComplete:
		tr.Outcome = "alpha concluded"
		tr.Evidence = "focused tests"
	case TransitionBlock:
		tr.Blocker = "waiting on upstream"
		tr.ResumeWhen = "upstream ships 1.2"
	case TransitionCancel, TransitionReopen:
		tr.Reason = "scope moved"
	}
	return tr
}

func TestTransitionMatrix(t *testing.T) {
	cases := []struct {
		from    PlanStatus
		action  string
		to      PlanStatus // empty means the transition must be refused
		allowed string     // expected allowed-actions list when refused
	}{
		{PlanPending, TransitionStart, PlanInProgress, ""},
		{PlanPending, TransitionComplete, PlanCompleted, ""},
		{PlanPending, TransitionBlock, PlanBlocked, ""},
		{PlanPending, TransitionResume, "", "start, complete, block, cancel"},
		{PlanPending, TransitionCancel, PlanCancelled, ""},
		{PlanPending, TransitionReopen, "", "start, complete, block, cancel"},
		{PlanInProgress, TransitionStart, "", "complete, block, cancel"},
		{PlanInProgress, TransitionComplete, PlanCompleted, ""},
		{PlanInProgress, TransitionBlock, PlanBlocked, ""},
		{PlanInProgress, TransitionResume, "", "complete, block, cancel"},
		{PlanInProgress, TransitionCancel, PlanCancelled, ""},
		{PlanInProgress, TransitionReopen, "", "complete, block, cancel"},
		{PlanBlocked, TransitionStart, "", "resume, cancel"},
		{PlanBlocked, TransitionComplete, "", "resume, cancel"},
		{PlanBlocked, TransitionBlock, "", "resume, cancel"},
		{PlanBlocked, TransitionResume, PlanInProgress, ""},
		{PlanBlocked, TransitionCancel, PlanCancelled, ""},
		{PlanBlocked, TransitionReopen, "", "resume, cancel"},
		{PlanCompleted, TransitionStart, "", "reopen"},
		{PlanCompleted, TransitionComplete, "", "reopen"},
		{PlanCompleted, TransitionBlock, "", "reopen"},
		{PlanCompleted, TransitionResume, "", "reopen"},
		{PlanCompleted, TransitionCancel, "", "reopen"},
		{PlanCompleted, TransitionReopen, PlanPending, ""},
		{PlanCancelled, TransitionStart, "", "reopen"},
		{PlanCancelled, TransitionComplete, "", "reopen"},
		{PlanCancelled, TransitionBlock, "", "reopen"},
		{PlanCancelled, TransitionResume, "", "reopen"},
		{PlanCancelled, TransitionCancel, "", "reopen"},
		{PlanCancelled, TransitionReopen, PlanPending, ""},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.from, tc.action), func(t *testing.T) {
			m := transitionFixture(t, tc.from)
			before := m.Plan()

			plan, result, err := m.TransitionPlan(transitionPayload(tc.action, "m1"), false)
			if tc.to == "" {
				require.ErrorContains(
					t,
					err,
					fmt.Sprintf("step %q is %s; allowed actions: %s", "alpha", tc.from, tc.allowed),
				)
				after := m.Plan()
				before.UpdatedAt = before.UpdatedAt.Round(0)
				after.UpdatedAt = after.UpdatedAt.Round(0)
				assert.Equal(t, before, after, "a refused transition changes nothing, not even the audit trail")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.from, result.From)
			assert.Equal(t, tc.to, result.To)
			assert.Equal(t, tc.action, result.Action)
			assert.Equal(t, "alpha", result.StepID)
			assert.Equal(t, uint64(3), plan.Revision)
			require.Len(t, plan.Items, 2)
			assert.Equal(t, tc.to, plan.Items[0].Status)
			require.Len(t, plan.Events, 1, "one audit event per transition")
			assert.Equal(t, tc.action, plan.Events[0].Action)
			assert.Equal(t, "m1", plan.Events[0].Mutation)
			assert.Equal(t, tc.from, plan.Events[0].From)
			assert.Equal(t, tc.to, plan.Events[0].To)
			assert.NotEmpty(t, result.EventID)
			assert.Equal(t, plan.Events[0].ID, result.EventID)
		})
	}
}

func TestTransitionCompleteEvidenceContract(t *testing.T) {
	cases := []struct {
		name    string
		tr      PlanTransition
		errText string
	}{
		{
			name:    "outcome is required",
			tr:      PlanTransition{Action: TransitionComplete, StepID: "alpha", MutationID: "c1", Evidence: "proof"},
			errText: `complete step "alpha": outcome is required`,
		},
		{
			name: "evidence or no-evidence reason is required",
			tr: PlanTransition{
				Action: TransitionComplete, StepID: "alpha", MutationID: "c1", Outcome: "shipped",
			},
			errText: `complete step "alpha": requires evidence, evidence_refs, or no_evidence_reason`,
		},
		{
			name: "no-evidence reason cannot ride evidence",
			tr: PlanTransition{
				Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
				Outcome: "shipped", Evidence: "proof", NoEvidenceReason: "unobservable",
			},
			errText: `complete step "alpha": no_evidence_reason is only allowed without evidence`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := transitionFixture(t, PlanInProgress)
			before := m.Plan()
			_, _, err := m.TransitionPlan(tc.tr, false)
			require.ErrorContains(t, err, tc.errText)
			after := m.Plan()
			before.UpdatedAt = before.UpdatedAt.Round(0)
			after.UpdatedAt = after.UpdatedAt.Round(0)
			assert.Equal(t, before, after)
		})
	}

	t.Run("evidence item satisfies the contract", func(t *testing.T) {
		m := transitionFixture(t, PlanInProgress)
		plan, _, err := m.TransitionPlan(PlanTransition{
			Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
			Outcome: "shipped", Evidence: "focused tests",
		}, false)
		require.NoError(t, err)
		assert.Equal(t, "shipped", plan.Items[0].Outcome)
		assert.Equal(t, "focused tests", plan.Items[0].Evidence)
		assert.Empty(t, plan.Items[0].EvidenceRefs)
	})

	t.Run("evidence refs satisfy the contract", func(t *testing.T) {
		m := transitionFixture(t, PlanInProgress)
		plan, _, err := m.TransitionPlan(PlanTransition{
			Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
			Outcome: "shipped", EvidenceRefs: []string{"internal/session/plan_transition.go"},
		}, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"internal/session/plan_transition.go"}, plan.Items[0].EvidenceRefs)
		assert.Empty(t, plan.Items[0].Evidence)
	})

	t.Run("no-evidence reason satisfies the contract", func(t *testing.T) {
		m := transitionFixture(t, PlanInProgress)
		plan, _, err := m.TransitionPlan(PlanTransition{
			Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
			Outcome: "unobservable step closed", NoEvidenceReason: "nothing to run",
		}, false)
		require.NoError(t, err)
		assert.Empty(t, plan.Items[0].Evidence)
		require.Len(t, plan.Events, 1)
		assert.Equal(t, "nothing to run", plan.Events[0].NoEvidenceReason)
	})
}

func TestTransitionRequiresReasons(t *testing.T) {
	cases := []struct {
		name    string
		from    PlanStatus
		tr      PlanTransition
		errText string
	}{
		{
			name:    "block without blocker",
			from:    PlanInProgress,
			tr:      PlanTransition{Action: TransitionBlock, StepID: "alpha", MutationID: "b1", ResumeWhen: "later"},
			errText: `block step "alpha": blocker is required`,
		},
		{
			name:    "block without resume condition",
			from:    PlanInProgress,
			tr:      PlanTransition{Action: TransitionBlock, StepID: "alpha", MutationID: "b1", Blocker: "upstream"},
			errText: `block step "alpha": resume_when is required`,
		},
		{
			name:    "cancel without reason",
			from:    PlanPending,
			tr:      PlanTransition{Action: TransitionCancel, StepID: "alpha", MutationID: "x1"},
			errText: `cancel step "alpha": reason is required`,
		},
		{
			name:    "reopen without reason",
			from:    PlanCompleted,
			tr:      PlanTransition{Action: TransitionReopen, StepID: "alpha", MutationID: "r1"},
			errText: `reopen step "alpha": reason is required`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := transitionFixture(t, tc.from)
			_, _, err := m.TransitionPlan(tc.tr, false)
			require.ErrorContains(t, err, tc.errText)
		})
	}
}

func TestTransitionBlockStoresAndResumeClearsReason(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, _, err := m.TransitionPlan(PlanTransition{
		Action: TransitionBlock, StepID: "alpha", MutationID: "b1",
		Blocker: "waiting on upstream", ResumeWhen: "upstream ships 1.2",
	}, false)
	require.NoError(t, err)
	assert.Equal(t, PlanBlocked, plan.Items[0].Status)
	assert.Equal(t, "waiting on upstream", plan.Items[0].Blocker)
	assert.Equal(t, "upstream ships 1.2", plan.Items[0].ResumeWhen)

	plan, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionResume, StepID: "alpha", MutationID: "rs1",
	}, false)
	require.NoError(t, err)
	assert.Equal(t, PlanInProgress, plan.Items[0].Status)
	assert.Empty(t, plan.Items[0].Blocker, "resume clears the blocker; it no longer waits")
	assert.Empty(t, plan.Items[0].ResumeWhen)
}

func TestTransitionReopenAuditsInsteadOfSilentlyRewriting(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	_, _, err := m.TransitionPlan(PlanTransition{
		Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
		Outcome: "shipped", Evidence: "focused tests", EvidenceRefs: []string{"internal/session/plan.go"},
	}, false)
	require.NoError(t, err)

	plan, _, err := m.TransitionPlan(PlanTransition{
		Action: TransitionReopen, StepID: "alpha", MutationID: "r1", Reason: "needs a second pass",
	}, false)
	require.NoError(t, err)
	assert.Equal(t, PlanPending, plan.Items[0].Status)
	assert.Empty(t, plan.Items[0].Outcome, "the step restarts clean")
	assert.Empty(t, plan.Items[0].Evidence)
	assert.Empty(t, plan.Items[0].EvidenceRefs)

	require.Len(t, plan.Events, 2, "the completion stays on the record")
	completion := plan.Events[0]
	assert.Equal(t, TransitionComplete, completion.Action)
	assert.Equal(t, "shipped", completion.Outcome)
	assert.Equal(t, "focused tests", completion.Evidence)
	assert.Equal(t, []string{"internal/session/plan.go"}, completion.EvidenceRefs)
	reopen := plan.Events[1]
	assert.Equal(t, TransitionReopen, reopen.Action)
	assert.Equal(t, "needs a second pass", reopen.Reason)
	assert.Equal(t, PlanCompleted, reopen.From)
	assert.Equal(t, PlanPending, reopen.To)
}

func TestTransitionIdempotentByMutationID(t *testing.T) {
	m := transitionFixture(t, PlanPending)

	plan1, result1, err := m.TransitionPlan(transitionPayload(TransitionStart, "m1"), false)
	require.NoError(t, err)
	require.Equal(t, uint64(3), plan1.Revision)

	plan2, result2, err := m.TransitionPlan(transitionPayload(TransitionStart, "m1"), false)
	require.NoError(t, err, "a retried mutation replays instead of failing")
	expected := result1
	expected.Replayed = true
	assert.Equal(t, expected, result2)
	assert.Equal(t, plan1.Revision, plan2.Revision, "a replay moves no revision")
	assert.Equal(t, uint64(3), plan2.Revision)
	require.Len(t, plan2.Events, 1, "a replay appends no duplicate event")
	require.Len(t, plan2.Mutations, 1)

	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionCancel, StepID: "alpha", MutationID: "m1", Reason: "collision",
	}, false)
	require.ErrorContains(
		t,
		err,
		`mutation id "m1" was already used for start step "alpha"`,
		"an id may not be reused for different work",
	)
}

func TestTransitionReplayLeavesNoDuplicateEvidence(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	complete := PlanTransition{
		Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
		Outcome: "shipped", Evidence: "focused tests", EvidenceRefs: []string{"internal/session/plan.go"},
	}

	_, _, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)

	plan, result, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)
	assert.True(t, result.Replayed)
	require.Len(t, plan.Items[0].EvidenceRefs, 1)
	assert.Equal(t, "focused tests", plan.Items[0].Evidence)
	require.Len(t, plan.Events, 1)
	assert.Equal(t, uint64(3), plan.Revision)
}

func TestTransitionReplaySurvivesReopen(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	complete := PlanTransition{
		Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
		Outcome: "shipped", Evidence: "focused tests",
	}
	_, _, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	plan, result, err := loaded.TransitionPlan(complete, false)
	require.NoError(t, err)
	assert.True(t, result.Replayed, "the mutation ledger is durable state")
	assert.Equal(t, uint64(3), plan.Revision)
}

func TestTransitionStartEnforcesSingleInProgress(t *testing.T) {
	contract := v2Fixture()
	contract.Items = []PlanItem{
		{ID: "alpha", Content: "first", Status: PlanPending, Type: StepEdit, Why: "w", DoneWhen: "d"},
		{ID: "beta", Content: "second", Status: PlanPending, Type: StepEdit, Why: "w", DoneWhen: "d"},
	}
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)

	_, _, err = m.TransitionPlan(transitionPayload(TransitionStart, "s1"), false)
	require.NoError(t, err)

	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionStart, StepID: "beta", MutationID: "s2",
	}, false)
	require.ErrorContains(t, err, `start step "beta"`)
	require.ErrorContains(t, err, "maximum is 1", "only one step may run at a time")
}

func TestTransitionAuditTrailIsBounded(t *testing.T) {
	m := transitionFixture(t, PlanPending)
	for i := range 30 {
		mutation := fmt.Sprintf("m%d", i)
		for _, action := range []string{TransitionStart, TransitionComplete, TransitionReopen} {
			_, _, err := m.TransitionPlan(transitionPayload(action, mutation+"-"+action), false)
			require.NoError(t, err)
		}
	}
	plan := m.Plan()
	assert.Len(t, plan.Events, maxPlanEvents, "the audit trail keeps a bounded tail")
	assert.Len(t, plan.Mutations, maxPlanEvents)
	assert.Equal(t, fmt.Sprintf("m29-%s", TransitionReopen), plan.Events[len(plan.Events)-1].Mutation)
}

func TestTransitionRequiresV2Plan(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, err = m.ReplacePlan([]PlanItem{{Content: "legacy step", Status: PlanPending, Type: StepEdit}})
	require.NoError(t, err)

	_, _, err = m.TransitionPlan(transitionPayload(TransitionStart, "m1"), false)
	require.ErrorContains(t, err, "session: plan transitions require a v2 plan")
}

func TestTransitionValidatesMutationID(t *testing.T) {
	cases := []struct {
		name     string
		mutation string
		errText  string
	}{
		{name: "empty", mutation: "", errText: "mutation id is required"},
		{name: "not a slug", mutation: "Bad ID!", errText: `mutation id "Bad ID!" must be a lowercase slug`},
		{
			name:     "overlong",
			mutation: strings.Repeat("m", maxPlanStepIDRunes+1),
			errText:  fmt.Sprintf("exceeds %d characters", maxPlanStepIDRunes),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := transitionFixture(t, PlanPending)
			tr := transitionPayload(TransitionStart, tc.mutation)
			_, _, err := m.TransitionPlan(tr, false)
			require.ErrorContains(t, err, tc.errText)
		})
	}
}

func TestTransitionRejectsUnknownActionAndMissingStep(t *testing.T) {
	m := transitionFixture(t, PlanPending)

	_, _, err := m.TransitionPlan(PlanTransition{
		Action: "teleport", StepID: "alpha", MutationID: "m1",
	}, false)
	require.ErrorContains(t, err, `unknown transition "teleport"`)
	require.ErrorContains(t, err, "start, complete, block, resume, cancel, reopen")

	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionStart, MutationID: "m1",
	}, false)
	require.ErrorContains(t, err, "step id is required")

	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionStart, StepID: "nope", MutationID: "m1",
	}, false)
	require.ErrorContains(t, err, `step "nope" not found`)
}

func TestTransitionApprovalFollowsOperationalPolicy(t *testing.T) {
	m := transitionFixture(t, PlanPending)

	plan, _, err := m.TransitionPlan(transitionPayload(TransitionStart, "s1"), false)
	require.NoError(t, err)
	assert.True(t, plan.Approved, "a status change keeps approval")

	plan, _, err = m.TransitionPlan(transitionPayload(TransitionComplete, "c1"), false)
	require.NoError(t, err)
	assert.False(t, plan.Approved, "finishing the last step closes approval")
}

func TestPatchPreservesTransitionHistory(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	complete := PlanTransition{
		Action: TransitionComplete, StepID: "alpha", MutationID: "c1",
		Outcome: "shipped", Evidence: "focused tests",
	}
	_, _, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)

	plan, _, err := m.PatchPlan(3, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "alpha", Note: pv("post-completion note")},
	}, false)
	require.NoError(t, err)
	require.Len(t, plan.Events, 1, "patching keeps the audit trail")
	require.Len(t, plan.Mutations, 1)

	_, result, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)
	assert.True(t, result.Replayed, "the mutation ledger survives patches")
}

func TestInsertStepClearsTransitionOwnedFields(t *testing.T) {
	m := transitionFixture(t, PlanPending)

	plan, _, err := m.PatchPlan(2, []PlanPatchOp{
		{
			Op:    PlanPatchInsertStep,
			After: "alpha",
			Step: &PlanItem{
				ID: "gamma", Content: "new step", Type: StepEdit,
				Why: "w", DoneWhen: "d",
				Status:     PlanInProgress,
				Blocker:    "smuggled blocker",
				ResumeWhen: "smuggled condition",
				Outcome:    "smuggled outcome",
				Evidence:   "smuggled evidence",
			},
		},
	}, false)
	require.NoError(t, err)
	inserted := plan.Items[1]
	assert.Equal(t, "gamma", inserted.ID)
	assert.Equal(t, PlanPending, inserted.Status)
	assert.Empty(t, inserted.Blocker)
	assert.Empty(t, inserted.ResumeWhen)
	assert.Empty(t, inserted.Outcome)
	assert.Empty(t, inserted.Evidence)
}

// TestTransitionForeignTableCoversEveryField pins transitionForeignFields to
// the struct: a field added to PlanTransition but forgotten in the table would
// be silently ignored by an action that does not take it.
func TestTransitionForeignTableCoversEveryField(t *testing.T) {
	owner := map[string]string{
		"Outcome":          TransitionComplete,
		"Evidence":         TransitionComplete,
		"EvidenceRefs":     TransitionComplete,
		"NoEvidenceReason": TransitionComplete,
		"Blocker":          TransitionBlock,
		"ResumeWhen":       TransitionBlock,
		"Reason":           TransitionCancel,
		"PlanResult":       TransitionComplete,
	}
	typ := reflect.TypeFor[PlanTransition]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Name == "Action" || field.Name == "StepID" || field.Name == "MutationID" {
			continue
		}
		stranger := reflect.ValueOf(&PlanTransition{Action: TransitionStart}).Elem()
		stranger.Field(i).Set(populatedValue(t, field.Type))
		require.NotEmptyf(
			t,
			transitionForeignFields(stranger.Interface().(PlanTransition)),
			"field %s must be visible to the foreign-field table",
			field.Name,
		)

		action := owner[field.Name]
		owned := reflect.ValueOf(&PlanTransition{Action: action}).Elem()
		owned.Field(i).Set(populatedValue(t, field.Type))
		require.Emptyf(
			t,
			transitionForeignFields(owned.Interface().(PlanTransition)),
			"%s must accept its own field %s",
			action,
			field.Name,
		)
	}
}

// finishFixture returns an approved v2 manager at revision 2 holding exactly
// the given items, so finish refusals can stage non-terminal neighbors the
// fixed transitionFixture never shows.
func finishFixture(t *testing.T, items ...PlanItem) *Manager {
	t.Helper()
	contract := v2Fixture()
	contract.Items = items
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)
	return m
}

// TestTransitionCompleteFinishesPlan covers the plan-tool road of the
// auto-finish: a complete that names plan_result closes the plan in the same
// write, and a retried mutation replays the close with it.
func TestTransitionCompleteFinishesPlan(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	tr := transitionPayload(TransitionComplete, "close-1")
	tr.PlanResult = PlanResultSuccess

	plan, result, err := m.TransitionPlan(tr, false)
	require.NoError(t, err)
	assert.Equal(t, PlanResultSuccess, plan.Result)
	require.NotNil(t, plan.ClosedAt)
	assert.Equal(t, uint64(3), plan.Revision)
	assert.Equal(t, PlanResultSuccess, result.PlanClosed)
	// The audit trail records the step move and the finish, in that order,
	// under the one mutation id.
	moves := plan.Events[len(plan.Events)-2:]
	assert.Equal(t, TransitionComplete, moves[0].Action)
	assert.Equal(t, TransitionFinish, moves[1].Action)
	assert.Equal(t, "close-1", moves[1].Mutation)

	replayPlan, replay, err := m.TransitionPlan(tr, false)
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, PlanResultSuccess, replay.PlanClosed)
	assert.Equal(t, plan.Revision, replayPlan.Revision)
	assert.Len(t, replayPlan.Events, len(plan.Events), "a replayed close adds no audit")
}

// TestTransitionCompletePlanResultRefusals pins every machine-checkable
// reason a close is refused: work still in flight, a success that would bury
// cancelled work, and an unknown result value.
func TestTransitionCompletePlanResultRefusals(t *testing.T) {
	item := func(id string, status PlanStatus) PlanItem {
		return PlanItem{
			ID: id, Content: "step " + id, Status: status, Type: StepEdit,
			Why: "finish semantics", DoneWhen: "recorded",
		}
	}
	closer := func(mutation string, result PlanResult) PlanTransition {
		tr := transitionPayload(TransitionComplete, mutation)
		tr.PlanResult = result
		return tr
	}

	t.Run("refuses while a neighbor is not terminal", func(t *testing.T) {
		m := finishFixture(t, item("alpha", PlanInProgress), item("beta", PlanPending))
		_, _, err := m.TransitionPlan(closer("close-p", PlanResultSuccess), false)
		require.ErrorContains(
			t, err, "plan_result refuses: 1 step(s) not terminal: beta (pending)",
		)
		assert.Empty(t, m.Plan().Result, "the refused close changed nothing")
		assert.Equal(t, PlanInProgress, m.Plan().Items[0].Status, "the step move rolled back too")
	})

	t.Run("success refuses to bury a cancelled step", func(t *testing.T) {
		m := finishFixture(t, item("alpha", PlanInProgress), item("beta", PlanCancelled))
		_, _, err := m.TransitionPlan(closer("close-c", PlanResultSuccess), false)
		require.ErrorContains(t, err, "plan_result success refuses: 1 cancelled step(s)")
	})

	t.Run("abandoned may close over cancelled work", func(t *testing.T) {
		m := finishFixture(t, item("alpha", PlanInProgress), item("beta", PlanCancelled))
		plan, result, err := m.TransitionPlan(closer("close-a", PlanResultAbandoned), false)
		require.NoError(t, err)
		assert.Equal(t, PlanResultAbandoned, plan.Result)
		assert.Equal(t, PlanResultAbandoned, result.PlanClosed)
	})

	t.Run("unknown plan_result is refused", func(t *testing.T) {
		m := finishFixture(t, item("alpha", PlanInProgress))
		_, _, err := m.TransitionPlan(closer("close-x", PlanResult("bogus")), false)
		require.ErrorContains(t, err, `plan_result must be "success" or "abandoned"`)
	})
}

// TestTransitionReopenClosedPlan covers the plan-level reopen: no step id and
// a reason restore a finished plan, keeping every step status it ended with.
func TestTransitionReopenClosedPlan(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	finish := transitionPayload(TransitionComplete, "close-1")
	finish.PlanResult = PlanResultAbandoned
	closed, _, err := m.TransitionPlan(finish, false)
	require.NoError(t, err)
	require.NotEmpty(t, closed.Result)

	plan, result, err := m.TransitionPlan(PlanTransition{
		Action: TransitionReopen, MutationID: "reopen-1", Reason: "one more landing",
	}, false)
	require.NoError(t, err)
	assert.Empty(t, plan.Result)
	assert.Nil(t, plan.ClosedAt)
	assert.Equal(t, uint64(4), plan.Revision)
	assert.Equal(t, TransitionReopen, result.Action)
	for _, item := range plan.Items {
		// Steps reopen individually; the plan-level reopen keeps their statuses.
		assert.Equal(t, PlanCompleted, item.Status, item.ID)
	}
	last := plan.Events[len(plan.Events)-1]
	assert.Equal(t, TransitionReopen, last.Action)
	assert.Equal(t, "one more landing", last.Reason)

	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionReopen, MutationID: "reopen-2", Reason: "again",
	}, false)
	require.ErrorContains(
		t, err, "reopen without id needs a finished plan; the plan is open",
	)
}

// roundPlanTimes canonicalizes a plan through the same JSON round-trip the
// durable file performs, so an in-memory plan compares equal to its reloaded
// twin on every host. Dropping the monotonic clock (Round(0)) is not enough:
// on a UTC host decoded timestamps carry a nil location while time.Now keeps
// the local one — two representations of the same instant that deep equality
// rightly calls different.
func roundPlanTimes(t testing.TB, plan *Plan) {
	t.Helper()
	raw, err := json.Marshal(*plan)
	if err != nil {
		t.Fatalf("round-trip plan: %v", err)
	}
	if err := json.Unmarshal(raw, plan); err != nil {
		t.Fatalf("round-trip plan: %v", err)
	}
}

func TestTransitionStateIsDurable(t *testing.T) {
	m := transitionFixture(t, PlanPending)
	steps := []struct {
		tr PlanTransition
		to PlanStatus
	}{
		{transitionPayload(TransitionStart, "s1"), PlanInProgress},
		{transitionPayload(TransitionComplete, "c1"), PlanCompleted},
		{transitionPayload(TransitionReopen, "r1"), PlanPending},
		{transitionPayload(TransitionBlock, "b1"), PlanBlocked},
		{transitionPayload(TransitionResume, "rs1"), PlanInProgress},
		{transitionPayload(TransitionCancel, "x1"), PlanCancelled},
	}
	for _, step := range steps {
		want, _, err := m.TransitionPlan(step.tr, false)
		require.NoError(t, err)
		require.Equal(t, step.to, want.Items[0].Status)
		loaded, err := OpenSession(m.File())
		require.NoError(t, err)
		roundPlanTimes(t, &want)
		assert.Equal(t, want, loaded.Plan(), "every status lands durably, terminal ones included")
	}
}
