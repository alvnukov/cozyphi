package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// approvedPatchFixture stores the v2 fixture as an auto-approved plan at
// revision 1, so every classification case starts from the same approved
// baseline and applies exactly one change through a public writer.
func approvedPatchFixture(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(v2Fixture(), true)
	require.NoError(t, err)
	require.True(t, m.Plan().Approved, "fixture must start approved")
	require.Equal(t, uint64(1), m.Plan().Revision)
	return m
}

// TestPatchApprovalClassification is the classification table for the patch
// path: contract fields revoke the user's approval and answer with their
// material diff, operational fields keep approval and answer with none. One
// case per field, applied through the public patch seam.
func TestPatchApprovalClassification(t *testing.T) {
	cases := []struct {
		name    string
		ops     []PlanPatchOp
		approve bool
		diff    []PlanMaterialChange
	}{
		{
			"wording-only why keeps approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Why: pv("reworded rationale")}},
			true,
			nil,
		},
		{
			"working context keeps approval",
			[]PlanPatchOp{{Op: PlanPatchReplaceContext, WorkingContext: pv("fresh scratchpad notes")}},
			true,
			nil,
		},
		{
			"note keeps approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Note: pv("progress note")}},
			true,
			nil,
		},
		{
			"no-op content set keeps approval",
			[]PlanPatchOp{
				{
					Op:      PlanPatchUpdateStep,
					ID:      "decode-legacy",
					Content: pv("decode legacy snapshots into the same canonical shape"),
				},
			},
			true,
			nil,
		},
		{
			"risk change revokes approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Risk: pv("sharper blast radius")}},
			false,
			[]PlanMaterialChange{{Target: "decode-legacy", Field: "risk", Change: MaterialChanged}},
		},
		{
			"content change revokes approval",
			[]PlanPatchOp{
				{Op: PlanPatchUpdateStep, ID: "decode-legacy", Content: pv("decode and migrate legacy snapshots")},
			},
			false,
			[]PlanMaterialChange{{Target: "decode-legacy", Field: "content", Change: MaterialChanged}},
		},
		{
			"done_when change revokes approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", DoneWhen: pv("a different exit condition")}},
			false,
			[]PlanMaterialChange{{Target: "decode-legacy", Field: "doneWhen", Change: MaterialChanged}},
		},
		{
			"goal change revokes approval",
			[]PlanPatchOp{{Op: PlanPatchSetPlanFields, Goal: pv("a different goal")}},
			false,
			[]PlanMaterialChange{{Target: "plan", Field: "goal", Change: MaterialChanged}},
		},
		{
			"approach change revokes approval",
			[]PlanPatchOp{{Op: PlanPatchSetPlanFields, Approach: pv("a different approach")}},
			false,
			[]PlanMaterialChange{{Target: "plan", Field: "approach", Change: MaterialChanged}},
		},
		{
			"added criterion revokes approval",
			[]PlanPatchOp{{Op: PlanPatchAddCriterion, Value: "approval survives only operational edits"}},
			false,
			[]PlanMaterialChange{{
				Target: "plan", Field: "successCriteria", Change: MaterialAdded,
				Detail: "approval survives only operational edits",
			}},
		},
		{
			"reworded criterion revokes approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateCriterion, From: "legacy files still load", To: "legacy files resume"}},
			false,
			[]PlanMaterialChange{
				{Target: "plan", Field: "successCriteria", Change: MaterialRemoved, Detail: "legacy files still load"},
				{Target: "plan", Field: "successCriteria", Change: MaterialAdded, Detail: "legacy files resume"},
			},
		},
		{
			"removed constraint revokes approval",
			[]PlanPatchOp{{Op: PlanPatchRemoveConstraint, Value: "no tool API switch in the expand phase"}},
			false,
			[]PlanMaterialChange{{
				Target: "plan", Field: "constraints", Change: MaterialRemoved,
				Detail: "no tool API switch in the expand phase",
			}},
		},
		{
			"inserted step revokes approval",
			[]PlanPatchOp{{Op: PlanPatchInsertStep, After: "write-schema", Step: &PlanItem{
				ID: "wire-frame", Content: "wire the approval classification", Type: StepExplore,
				Why: "the table needs a consumer", DoneWhen: "tests pin every field",
			}}},
			false,
			[]PlanMaterialChange{{Target: "wire-frame", Field: "step", Change: MaterialAdded}},
		},
		{
			"reordered steps revoke approval",
			[]PlanPatchOp{{Op: PlanPatchReorderSteps, IDs: []string{"decode-legacy", "write-schema"}}},
			false,
			[]PlanMaterialChange{{Target: "plan", Field: "steps", Change: MaterialReordered}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := approvedPatchFixture(t)
			patched, summary, err := m.PatchPlan(1, tc.ops, false)
			require.NoError(t, err)
			assert.Equal(t, tc.approve, patched.Approved)
			assert.Equal(t, tc.diff, summary.Diff, "the summary carries the exact material diff")
		})
	}
}

// TestRemovePendingStepRevokesApproval covers the one structural op the
// classification table cannot reach from the fixture: removing a step needs a
// pending step, so the case inserts one first with auto-approval on.
func TestRemovePendingStepRevokesApproval(t *testing.T) {
	m := approvedPatchFixture(t)
	_, _, err := m.PatchPlan(1, []PlanPatchOp{{Op: PlanPatchInsertStep, After: "write-schema", Step: &PlanItem{
		ID: "wire-frame", Content: "wire the approval classification", Type: StepExplore,
		Why: "the table needs a consumer", DoneWhen: "tests pin every field",
	}}}, true)
	require.NoError(t, err)
	require.True(t, m.Plan().Approved)

	patched, summary, err := m.PatchPlan(2, []PlanPatchOp{{Op: PlanPatchRemoveStep, ID: "wire-frame"}}, false)
	require.NoError(t, err)
	assert.False(t, patched.Approved)
	assert.Equal(t, []PlanMaterialChange{{Target: "wire-frame", Field: "step", Change: MaterialRemoved}}, summary.Diff)
}

// TestReplaceApprovalClassification pins the same table through the replace
// path, the only public writer that can move step type and the JIT posture.
func TestReplaceApprovalClassification(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*PlanV2)
		approve bool
		diff    []PlanMaterialChange
	}{
		{
			"identical replace keeps approval",
			func(*PlanV2) {},
			true,
			nil,
		},
		{
			"wording-only why keeps approval",
			func(c *PlanV2) { c.Items[1].Why = "reworded rationale" },
			true,
			nil,
		},
		{
			"working context keeps approval",
			func(c *PlanV2) { c.WorkingContext = "fresh scratchpad notes" },
			true,
			nil,
		},
		{
			"risk change revokes approval",
			func(c *PlanV2) { c.Items[1].Risk = "sharper blast radius" },
			false,
			[]PlanMaterialChange{{Target: "decode-legacy", Field: "risk", Change: MaterialChanged}},
		},
		{
			"type change revokes approval",
			func(c *PlanV2) { c.Items[1].Type = StepEdit },
			false,
			[]PlanMaterialChange{{
				Target: "decode-legacy", Field: "type", Change: MaterialChanged, Detail: "run to edit",
			}},
		},
		{
			"jit flip revokes approval",
			func(c *PlanV2) { c.Items[1].JIT = false },
			false,
			[]PlanMaterialChange{{Target: "decode-legacy", Field: "jit", Change: MaterialChanged}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := approvedPatchFixture(t)
			contract := v2Fixture()
			tc.mutate(&contract)
			replaced, diff, err := m.ReplacePlanV2(contract, false)
			require.NoError(t, err)
			assert.Equal(t, tc.approve, replaced.Approved)
			assert.Equal(t, tc.diff, diff, "the replace answers with the exact material diff")
		})
	}
}

// TestReplacePlanV2ReturnsMaterialDiff pins the deterministic diff of a
// first create against the empty plan, and of an amended re-create against
// the approved one.
func TestReplacePlanV2ReturnsMaterialDiff(t *testing.T) {
	m := NewManager(t.TempDir())
	_, diff, err := m.ReplacePlanV2(v2Fixture(), false)
	require.NoError(t, err)
	assert.Equal(t, []PlanMaterialChange{
		{Target: "plan", Field: "goal", Change: MaterialChanged},
		{Target: "plan", Field: "approach", Change: MaterialChanged},
		{Target: "plan", Field: "successCriteria", Change: MaterialAdded, Detail: "round-trip keeps every field"},
		{Target: "plan", Field: "successCriteria", Change: MaterialAdded, Detail: "legacy files still load"},
		{Target: "plan", Field: "constraints", Change: MaterialAdded, Detail: "no tool API switch in the expand phase"},
		{Target: "write-schema", Field: "step", Change: MaterialAdded},
		{Target: "decode-legacy", Field: "step", Change: MaterialAdded},
	}, diff)

	_, err = m.SetPlanApproved(true)
	require.NoError(t, err)

	amended := v2Fixture()
	amended.Goal = "a different goal"
	amended.Items[0].Content = "extend Plan with the v2 contract fields"
	replaced, diff, err := m.ReplacePlanV2(amended, false)
	require.NoError(t, err)
	assert.False(t, replaced.Approved)
	assert.Equal(t, []PlanMaterialChange{
		{Target: "plan", Field: "goal", Change: MaterialChanged},
		{Target: "write-schema", Field: "content", Change: MaterialChanged},
	}, diff)
}

// TestMaterialChangeAndApprovalRestoreAfterResume proves the material batch
// and the approval change land in one durable entry and survive a reload —
// both the harness-side revocation and the user-side grant.
func TestMaterialChangeAndApprovalRestoreAfterResume(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(v2Fixture(), true)
	require.NoError(t, err)

	_, summary, err := m.PatchPlan(1, []PlanPatchOp{
		{Op: PlanPatchSetPlanFields, Goal: pv("a different goal")},
	}, false)
	require.NoError(t, err)
	require.Len(t, summary.Diff, 1)
	require.False(t, m.Plan().Approved)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, uint64(2), restored.Revision)
	assert.False(t, restored.Approved, "the revoked approval restores after resume")
	assert.Equal(t, "a different goal", restored.Goal, "the material batch restores after resume")

	_, err = loaded.SetPlanApproved(true)
	require.NoError(t, err)
	reloaded, err := OpenSession(loaded.File())
	require.NoError(t, err)
	assert.True(t, reloaded.Plan().Approved, "the user's grant restores after a second resume")
	assert.Equal(t, uint64(3), reloaded.Plan().Revision)
}

// TestTransitionOperationalFieldsKeepApproval completes the classification
// table for the fields only transitions can write: status, outcome, evidence,
// evidence refs, blocker, and resume condition all keep the user's approval.
func TestTransitionOperationalFieldsKeepApproval(t *testing.T) {
	contract := PlanV2{
		Goal:            "prove transitions stay operational",
		Approach:        "walk every field family through the state machine",
		SuccessCriteria: []string{"approval survives each move"},
		Items: []PlanItem{
			{ID: "alpha", Content: "first", Status: PlanPending, Type: StepExplore, Why: "w", DoneWhen: "d"},
			{ID: "beta", Content: "second", Status: PlanPending, Type: StepExplore, Why: "w", DoneWhen: "d"},
		},
	}
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(contract, true)
	require.NoError(t, err)
	require.True(t, m.Plan().Approved)

	plan, _, err := m.TransitionPlan(PlanTransition{
		Action: TransitionStart, StepID: "alpha", MutationID: "t-start",
	}, false)
	require.NoError(t, err)
	assert.True(t, plan.Approved, "a status change keeps approval")

	plan, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionComplete, StepID: "alpha", MutationID: "t-complete",
		Outcome: "outcome recorded", Evidence: "evidence recorded", EvidenceRefs: []string{"ref-1"},
	}, false)
	require.NoError(t, err)
	assert.True(t, plan.Approved, "outcome and evidence keep approval while work remains")

	plan, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionBlock, StepID: "beta", MutationID: "t-block",
		Blocker: "waiting on review", ResumeWhen: "review lands",
	}, false)
	require.NoError(t, err)
	assert.True(t, plan.Approved, "blocker and resume condition keep approval")

	plan, _, err = m.TransitionPlan(PlanTransition{
		Action: TransitionResume, StepID: "beta", MutationID: "t-resume",
	}, false)
	require.NoError(t, err)
	assert.True(t, plan.Approved, "resuming keeps approval")
}

// TestMaterialDiffLabelsLegacyStepsByOrdinal covers the idless legacy
// pairing: position labels stand in for stable ids.
func TestMaterialDiffLabelsLegacyStepsByOrdinal(t *testing.T) {
	old := Plan{Items: []PlanItem{
		{Content: "first", Status: PlanInProgress, Type: StepExplore},
		{Content: "second", Status: PlanPending, Type: StepEdit},
	}}
	changed := old.Clone()
	changed.Items[0].Content = "reworded first"
	assert.Equal(t, []PlanMaterialChange{
		{Target: "step 1", Field: "content", Change: MaterialChanged},
	}, MaterialDiff(old, changed))

	appended := old.Clone()
	appended.Items = append(appended.Items, PlanItem{Content: "third", Status: PlanPending, Type: StepRun})
	assert.Equal(t, []PlanMaterialChange{
		{Target: "step 3", Field: "step", Change: MaterialAdded},
	}, MaterialDiff(old, appended))
}
