package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jitFixture is a v2 contract whose tail steps carry irreversible external
// effects: publishing the tag and mailing users cannot be undone, so plan
// approval alone must not let them run.
func jitFixture() PlanV2 {
	return PlanV2{
		Goal:            "ship the release",
		Approach:        "verify, publish, announce",
		SuccessCriteria: []string{"the tag is on origin"},
		Items: []PlanItem{
			{
				ID: "run-suite", Content: "run the release suite", Status: PlanInProgress,
				Type: StepRun, Why: "gates the release", DoneWhen: "suite is green",
			},
			{
				ID: "push-tag", Content: "push the release tag", Status: PlanPending,
				Type: StepRun, Why: "publishes the build", DoneWhen: "tag is on origin",
				Risk: "a published tag is irreversible", JIT: true,
			},
			{
				ID: "notify-users", Content: "send the announcement", Status: PlanPending,
				Type: StepIntegrate, Why: "users wait on the release", DoneWhen: "mail is sent",
				Risk: "sent mail cannot be unsent", JIT: true,
			},
		},
	}
}

func newJITManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	return m
}

func TestSetStepJITApprovedGrantsStepAtContractEpoch(t *testing.T) {
	m := newJITManager(t)
	plan, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	require.True(t, plan.Approved)

	granted, err := m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)
	assert.True(t, granted.JITGranted("push-tag"), "the step the user approved is granted")
	assert.False(t, granted.JITGranted("notify-users"), "a grant never crosses steps")
	assert.True(t, granted.Approved, "a step grant does not touch plan approval")
	assert.Greater(t, granted.Revision, plan.Revision)
	assert.Equal(t, plan.ContractEpoch, granted.ContractEpoch, "granting moves no contract epoch")
}

func TestJITGrantSurvivesOperationalWrites(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	granted, err := m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)
	epoch := granted.ContractEpoch

	// Attempts and transitions are operational evidence the harness writes
	// as work proceeds; they must not expire the user's grant.
	_, err = m.RecordPlanAttempt("push-tag", PlanAttempt{
		CallID: "c1", Tool: "bash", Status: AttemptSuccess, Summary: "tag pushed",
	})
	require.NoError(t, err)
	_, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionComplete, StepID: "run-suite", MutationID: "m1",
		Outcome: "suite green", Evidence: "tests pass",
	}, false)
	require.NoError(t, err)

	assert.True(t, m.Plan().JITGranted("push-tag"), "operational writes keep the grant")
	assert.Equal(t, epoch, m.Plan().ContractEpoch, "operational writes move no epoch")
}

func TestJITGrantDiesOnMaterialChangeAndStaysDead(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	granted, err := m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)

	// A reopened step with a changed action is a different promise: the old
	// grant dies with the contract it approved.
	_, _, err = m.PatchPlan(granted.Revision, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "push-tag", Content: pv("push the release tag now")},
	}, false)
	require.NoError(t, err)
	assert.False(t, m.Plan().Approved, "the content change is material")
	assert.False(t, m.Plan().JITGranted("push-tag"), "the grant died with the contract")

	reapproved, err := m.SetPlanApproved(true)
	require.NoError(t, err)
	assert.True(t, reapproved.Approved)
	assert.False(t, reapproved.JITGranted("push-tag"), "re-approving the plan revives no step grant")
}

func TestSetStepJITApprovedValidatesItsStep(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)

	_, err = m.SetStepJITApproved("missing-step", true)
	require.ErrorContains(t, err, "missing-step")

	_, err = m.SetStepJITApproved("run-suite", true)
	require.ErrorContains(t, err, "not marked just-in-time")
}

func TestSetStepJITApprovedWithdrawsGrant(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	_, err = m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)

	withdrawn, err := m.SetStepJITApproved("push-tag", false)
	require.NoError(t, err)
	assert.False(t, withdrawn.JITGranted("push-tag"))

	// Withdrawing a grant that is already gone is a no-op, not an error:
	// the safe state needs no ceremony.
	_, err = m.SetStepJITApproved("push-tag", false)
	require.NoError(t, err)
}

func TestJITGrantSurvivesResume(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	granted, err := m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.True(t, restored.JITGranted("push-tag"), "the user's step grant restores after resume")
	assert.Equal(t, granted.ContractEpoch, restored.ContractEpoch)
}

func TestJITGrantDiesOnStepTypeRename(t *testing.T) {
	m := newJITManager(t)
	_, _, err := m.ReplacePlanV2(jitFixture(), true)
	require.NoError(t, err)
	_, err = m.SetStepJITApproved("push-tag", true)
	require.NoError(t, err)

	renamed, err := m.RenamePlanStepTypes(map[StepType]StepType{StepRun: "execute"})
	require.NoError(t, err)
	assert.True(t, renamed.Approved, "the rename transaction keeps plan approval")
	assert.False(t, renamed.JITGranted("push-tag"), "a type rewrite expires the step grant")
}

func TestOpenSessionRejectsBogusJITApprovals(t *testing.T) {
	cases := []struct {
		name    string
		grants  []JITApproval
		wantErr string
	}{
		{
			name:    "step id must be a slug",
			grants:  []JITApproval{{StepID: "Not A Slug", Epoch: 1}},
			wantErr: "just-in-time",
		},
		{
			name: "one grant per step",
			grants: []JITApproval{
				{StepID: "push-tag", Epoch: 1},
				{StepID: "push-tag", Epoch: 2},
			},
			wantErr: "duplicate",
		},
		{
			name:    "grants name plan steps only",
			grants:  []JITApproval{{StepID: "other-step", Epoch: 1}},
			wantErr: "unknown step",
		},
		{
			name:    "a future epoch is a standing pre-approval",
			grants:  []JITApproval{{StepID: "push-tag", Epoch: 2}},
			wantErr: "future contract epoch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newJITManager(t)
			now := time.Now()
			entry := PlanEntry{
				SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: m.generateID(), Timestamp: now},
				Plan: Plan{
					Schema: PlanSchemaV2, UpdatedAt: now, ContractEpoch: 1, Approved: true,
					Items: []PlanItem{{
						ID: "push-tag", Content: "push the release tag", Status: PlanPending,
						Type: StepRun, Why: "publishes", DoneWhen: "tag is on origin", JIT: true,
					}},
					JITApprovals: tc.grants,
				},
			}
			m.entries = append(m.entries, entry)
			m.byIDs[entry.ID] = entry
			require.NoError(t, m.flush(entry))

			_, err := OpenSession(m.File())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
