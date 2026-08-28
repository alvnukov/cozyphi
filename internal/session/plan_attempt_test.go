package session

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordPlanAttemptWritesBoundedDurableRecord: one accepted call lands as
// exactly one attempt on the addressed step, with a harness-truncated summary
// that never duplicates raw tool output in the plan.
func TestRecordPlanAttemptWritesBoundedDurableRecord(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, err := m.RecordPlanAttempt("alpha", PlanAttempt{
		CallID:  "toolu_raw01",
		Tool:    "write",
		Status:  AttemptSuccess,
		Summary: strings.Repeat("x", 4096),
	})
	require.NoError(t, err)

	item := plan.Items[0]
	require.Len(t, item.Attempts, 1)
	got := item.Attempts[0]
	assert.Equal(t, "toolu_raw01", got.CallID)
	assert.Equal(t, "write", got.Tool)
	assert.Equal(t, AttemptSuccess, got.Status)
	assert.LessOrEqual(t, len([]rune(got.Summary)), maxPlanAttemptSummaryRunes)
	assert.True(t, strings.HasPrefix(got.Summary, "xxx"), "the summary is a prefix of the output")
	assert.False(t, got.At.IsZero(), "the write stamps the terminal time")
	assert.Equal(t, uint64(3), plan.Revision, "one durable write, one revision")

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	require.Len(t, loaded.Plan().Items[0].Attempts, 1, "the attempt survives resume")
}

// TestRecordPlanAttemptUpsertsByCallID: reconciliation that re-reports the
// same call updates the record in place instead of duplicating it.
func TestRecordPlanAttemptUpsertsByCallID(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	_, err := m.RecordPlanAttempt(
		"alpha",
		PlanAttempt{CallID: "c1", Tool: "write", Status: AttemptSuccess, Summary: "ok"},
	)
	require.NoError(t, err)
	plan, err := m.RecordPlanAttempt(
		"alpha",
		PlanAttempt{CallID: "c1", Tool: "write", Status: AttemptLost, Summary: "ok"},
	)
	require.NoError(t, err)

	require.Len(t, plan.Items[0].Attempts, 1, "the same call id is one record")
	assert.Equal(t, AttemptLost, plan.Items[0].Attempts[0].Status)
	assert.Equal(t, uint64(4), plan.Revision, "two writes, two revisions, one record")
}

// TestRecordPlanAttemptStatusVocabulary: success, failure, cancellation and a
// lost result are the whole status set; anything else fails closed.
func TestRecordPlanAttemptStatusVocabulary(t *testing.T) {
	for _, status := range []string{AttemptSuccess, AttemptFailed, AttemptCanceled, AttemptLost} {
		m := transitionFixture(t, PlanInProgress)
		_, err := m.RecordPlanAttempt("alpha", PlanAttempt{CallID: "c1", Tool: "bash", Status: status})
		assert.NoError(t, err, status)
	}

	m := transitionFixture(t, PlanInProgress)
	_, err := m.RecordPlanAttempt("alpha", PlanAttempt{CallID: "c1", Tool: "bash", Status: "flaky"})
	assert.ErrorContains(t, err, `unknown attempt status "flaky"`)
}

// TestRecordPlanAttemptBoundedTail: the per-step history keeps only the most
// recent attempts, so a long-lived step stays bounded.
func TestRecordPlanAttemptBoundedTail(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	for i := range maxPlanAttemptsPerStep + 2 {
		_, err := m.RecordPlanAttempt("alpha", PlanAttempt{
			CallID:  fmt.Sprintf("c%d", i),
			Tool:    "read",
			Status:  AttemptSuccess,
			Summary: strconv.Itoa(i),
		})
		require.NoError(t, err)
	}

	attempts := m.Plan().Items[0].Attempts
	require.Len(t, attempts, maxPlanAttemptsPerStep)
	assert.Equal(t, "c2", attempts[0].CallID, "the oldest attempts dropped off")
	assert.Equal(t, fmt.Sprintf("c%d", maxPlanAttemptsPerStep+1), attempts[len(attempts)-1].CallID)
}

// TestRecordPlanAttemptMovesNoLifecycleState: an erroneous attempt is audit
// trail, not a transition — the step keeps its status and the plan keeps the
// user's approval.
func TestRecordPlanAttemptMovesNoLifecycleState(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, err := m.RecordPlanAttempt(
		"alpha",
		PlanAttempt{CallID: "c1", Tool: "write", Status: AttemptFailed, Summary: "boom"},
	)
	require.NoError(t, err)
	assert.Equal(t, PlanInProgress, plan.Items[0].Status)
	assert.True(t, plan.Approved, "operational evidence never revokes approval")
}

// TestRecordPlanAttemptRequiresV2KnownStep: attempts need the same addressing
// contract as transitions — a v2 plan and a stable step id.
func TestRecordPlanAttemptRequiresV2KnownStep(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	_, err := m.RecordPlanAttempt("ghost", PlanAttempt{CallID: "c1", Tool: "read", Status: AttemptSuccess})
	assert.ErrorContains(t, err, `step "ghost" not found`)

	dir := t.TempDir()
	legacy, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, err = legacy.ReplacePlan([]PlanItem{{Content: "legacy step", Status: PlanInProgress, Type: StepEdit}})
	require.NoError(t, err)
	_, err = legacy.RecordPlanAttempt("anything", PlanAttempt{CallID: "c1", Tool: "read", Status: AttemptSuccess})
	assert.ErrorContains(t, err, "require a v2 plan")
}

// TestRecordPlanAttemptValidatesIdentity: the call id and tool name are the
// attempt's address into the transcript; both must be present and bounded.
func TestRecordPlanAttemptValidatesIdentity(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	_, err := m.RecordPlanAttempt("alpha", PlanAttempt{Tool: "read", Status: AttemptSuccess})
	assert.ErrorContains(t, err, "call id is required")

	_, err = m.RecordPlanAttempt("alpha", PlanAttempt{
		CallID: strings.Repeat("c", maxPlanAttemptCallIDRunes+1),
		Tool:   "read",
		Status: AttemptSuccess,
	})
	assert.ErrorContains(t, err, "call id exceeds")
}

// TestCreateStripsModelAuthoredAttempts: attempts are harness-recorded
// evidence; a contract that arrives carrying them loses them durably.
func TestCreateStripsModelAuthoredAttempts(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	contract := v2Fixture()
	contract.Items[0].Attempts = []PlanAttempt{{CallID: "fake", Tool: "read", Status: AttemptSuccess}}
	plan, _, err := m.ReplacePlanV2(contract, false)
	require.NoError(t, err)
	assert.Empty(t, plan.Items[0].Attempts)
}

// TestTransitionKeepsRecordedAttempts: the transition revalidation path
// restores harness-recorded attempts exactly as it restores the audit ledger.
func TestTransitionKeepsRecordedAttempts(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	_, err := m.RecordPlanAttempt(
		"alpha",
		PlanAttempt{CallID: "c1", Tool: "write", Status: AttemptSuccess, Summary: "ok"},
	)
	require.NoError(t, err)

	plan, _, err := m.TransitionPlan(transitionPayload(TransitionBlock, "m1"), false)
	require.NoError(t, err)
	require.Len(t, plan.Items[0].Attempts, 1)
}

// TestLoadedAttemptBoundsFailClosed: a snapshot this harness could not have
// written — oversized attempt fields — refuses to load.
func TestLoadedAttemptBoundsFailClosed(t *testing.T) {
	oversized := Plan{
		Schema:   PlanSchemaV2,
		Goal:     "g",
		Approach: "a",
		Items: []PlanItem{
			{
				ID:       "alpha",
				Content:  "c",
				Status:   PlanInProgress,
				Type:     StepEdit,
				Why:      "w",
				DoneWhen: "d",
				Attempts: []PlanAttempt{
					{CallID: "c1", Tool: "read", Status: AttemptSuccess, Summary: strings.Repeat("s", 512)},
				},
			},
		},
	}
	_, err := normalizeLoadedPlan(oversized)
	assert.ErrorContains(t, err, "attempt 1 summary exceeds")
}

// TestCompleteValidatesAttemptRefs: a call: evidence ref must name a
// successful attempt persisted on the completing step; other refs are
// model-authored artifacts and pass through untouched.
func TestCompleteValidatesAttemptRefs(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)
	seedAttempts(t, m)

	complete := PlanTransition{
		Action:       TransitionComplete,
		StepID:       "alpha",
		MutationID:   "c1",
		Outcome:      "shipped",
		EvidenceRefs: []string{attemptRefPrefix + "call_good", "internal/session/plan.go"},
	}
	plan, _, err := m.TransitionPlan(complete, false)
	require.NoError(t, err)
	assert.Equal(t, complete.EvidenceRefs, plan.Items[0].EvidenceRefs)
}

// TestCompleteRejectsForeignAndMissingAttemptRefs: refs naming an attempt the
// step never persisted — missing outright, failed, or recorded on another
// step — are refused with the ref named.
func TestCompleteRejectsForeignAndMissingAttemptRefs(t *testing.T) {
	cases := []struct {
		name string
		ref  string
	}{
		{"missing", attemptRefPrefix + "call_missing"},
		{"failed attempt", attemptRefPrefix + "call_bad"},
		{"foreign step", attemptRefPrefix + "call_beta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := transitionFixture(t, PlanInProgress)
			seedAttempts(t, m)

			_, _, err := m.TransitionPlan(PlanTransition{
				Action:       TransitionComplete,
				StepID:       "alpha",
				MutationID:   "c1",
				Outcome:      "shipped",
				EvidenceRefs: []string{tc.ref},
			}, false)
			assert.ErrorContains(
				t,
				err,
				fmt.Sprintf("evidence ref %q is not a successful attempt of this step", tc.ref),
			)
		})
	}
}

// seedAttempts gives step alpha one successful and one failed attempt, and
// step beta a successful one, so every kind of wrong ref is addressable.
func seedAttempts(t *testing.T, m *Manager) {
	t.Helper()
	_, err := m.RecordPlanAttempt(
		"alpha",
		PlanAttempt{CallID: "call_good", Tool: "bash", Status: AttemptSuccess, Summary: "tests green"},
	)
	require.NoError(t, err)
	_, err = m.RecordPlanAttempt("alpha", PlanAttempt{CallID: "call_bad", Tool: "bash", Status: AttemptFailed})
	require.NoError(t, err)
	_, err = m.RecordPlanAttempt("beta", PlanAttempt{CallID: "call_beta", Tool: "read", Status: AttemptSuccess})
	require.NoError(t, err)
}

// TestPatchInsertStepStripsAttempts: a step inserted by patch starts with no
// evidence — attempts are harness-recorded, so a forged citation can never
// resolve.
func TestPatchInsertStepStripsAttempts(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, _, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{{
		Op:     PlanPatchInsertStep,
		Before: "alpha",
		Step: &PlanItem{
			ID: "forged", Content: "smuggled", Type: StepEdit, Why: "w", DoneWhen: "d",
			Attempts: []PlanAttempt{{CallID: "call_fake", Tool: "read", Status: AttemptSuccess}},
		},
	}}, false)
	require.NoError(t, err)

	var inserted PlanItem
	for _, item := range plan.Items {
		if item.ID == "forged" {
			inserted = item
		}
	}
	require.Equal(t, "forged", inserted.ID)
	assert.Empty(t, inserted.Attempts, "no authoring path may seed attempt evidence")
}

// TestRecordPlanAttemptKeepsDeepOutputOutOfThePlan: the serialized snapshot
// carries only the bounded prefix of tool output — a secret fixture deep in
// the output never lands in the plan.
func TestRecordPlanAttemptKeepsDeepOutputOutOfThePlan(t *testing.T) {
	m := transitionFixture(t, PlanInProgress)

	plan, err := m.RecordPlanAttempt("alpha", PlanAttempt{
		CallID:  "c1",
		Tool:    "bash",
		Status:  AttemptSuccess,
		Summary: strings.Repeat("pad ", 4096) + "TOPSECRET-token",
	})
	require.NoError(t, err)

	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "TOPSECRET-token", "output past the bound stays in the transcript only")
	assert.Less(t, len(encoded), 4096, "the serialized plan stays small")
}
