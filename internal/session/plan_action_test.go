package session

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actionFixture extends the v2 fixture with plan actions, a per-step-type
// model map, step actions, and a step model override.
func actionFixture() PlanV2 {
	fixture := v2Fixture()
	fixture.Actions = []PlanAction{
		{Event: PlanActionOnPlanStart, Type: PlanActionCompact},
	}
	fixture.ModelsByType = map[StepType]string{
		StepEdit: "opus",
		StepRun:  "sonnet",
	}
	fixture.Items[1].Actions = []PlanAction{
		{Event: PlanActionOnStepStart, Type: PlanActionInjectSkill, Skills: []string{"tdd", "code-review"}},
		{Event: PlanActionOnStepEnd, Type: PlanActionCompact},
	}
	fixture.Items[1].Model = "haiku"
	return fixture
}

// pvActions and pvModels are the typed set-slot builders for the automation
// patch fields; the string pv covers prose fields only.
func pvActions(actions []PlanAction) PatchValue[[]PlanAction] {
	return PatchValue[[]PlanAction]{Set: true, Value: actions}
}

func pvModels(models map[StepType]string) PatchValue[map[StepType]string] {
	return PatchValue[map[StepType]string]{Set: true, Value: models}
}

func TestReplacePlanV2RoundTripsActionsAndModels(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	created, _, err := m.ReplacePlanV2(actionFixture(), false)
	require.NoError(t, err)
	assert.Equal(t, []PlanAction{{Event: PlanActionOnPlanStart, Type: PlanActionCompact}}, created.Actions)
	assert.Equal(t, map[StepType]string{StepEdit: "opus", StepRun: "sonnet"}, created.ModelsByType)
	assert.Equal(t, []PlanAction{
		{Event: PlanActionOnStepStart, Type: PlanActionInjectSkill, Skills: []string{"tdd", "code-review"}},
		{Event: PlanActionOnStepEnd, Type: PlanActionCompact},
	}, created.Items[1].Actions)
	assert.Equal(t, "haiku", created.Items[1].Model)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	restored := loaded.Plan()
	assert.Equal(t, created.Actions, restored.Actions)
	assert.Equal(t, created.ModelsByType, restored.ModelsByType)
	assert.Equal(t, created.Items, restored.Items, "reloading must be stable")
}

func TestReplacePlanV2StripsSeededRuns(t *testing.T) {
	m := NewManager(t.TempDir())
	fixture := actionFixture()
	fixture.Items[1].Actions[0].Runs = []PlanActionRun{{Status: PlanActionRunOK}}
	fixture.Actions[0].Runs = []PlanActionRun{{Status: PlanActionRunFailed, Error: "nope"}}

	created, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err)
	assert.Empty(t, created.Items[1].Actions[0].Runs, "authoring cannot seed run history")
	assert.Empty(t, created.Actions[0].Runs, "authoring cannot seed run history")
}

func TestReplacePlanV2ValidatesActions(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, err := m.ReplacePlanV2(actionFixture(), false)
	require.NoError(t, err)

	stepAction := func(mutate func(*PlanAction)) PlanV2 {
		fixture := actionFixture()
		if mutate != nil {
			mutate(&fixture.Items[1].Actions[0])
		}
		return fixture
	}
	cases := map[string]PlanV2{
		"unknown step event": stepAction(func(a *PlanAction) { a.Event = "step_middle" }),
		"plan event on step": stepAction(func(a *PlanAction) { a.Event = PlanActionOnPlanStart }),
		"unknown action type": stepAction(func(a *PlanAction) {
			a.Type = "run_tests"
		}),
		"compact with skills": stepAction(func(a *PlanAction) {
			a.Type = PlanActionCompact
			a.Skills = []string{"tdd"}
		}),
		"inject without skills":        stepAction(func(a *PlanAction) { a.Skills = nil }),
		"inject with blank skill":      stepAction(func(a *PlanAction) { a.Skills = []string{" "} }),
		"inject with duplicate skills": stepAction(func(a *PlanAction) { a.Skills = []string{"tdd", "tdd"} }),
		"inject with too many skills": stepAction(func(a *PlanAction) {
			a.Skills = []string{"a", "b", "c", "d", "e"}
		}),
		"skill too long": stepAction(func(a *PlanAction) {
			a.Skills = []string{strings.Repeat("s", maxPlanActionSkillRunes+1)}
		}),
		"empty event": stepAction(func(a *PlanAction) { a.Event = "" }),
		"empty type":  stepAction(func(a *PlanAction) { a.Type = "" }),
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := m.ReplacePlanV2(fixture, false)
			assert.Error(t, err, "invalid step action must fail closed")
		})
	}

	planCases := map[string]PlanV2{
		"plan action with step event": func() PlanV2 {
			fixture := actionFixture()
			fixture.Actions[0].Event = PlanActionOnStepStart
			return fixture
		}(),
		"too many plan actions": func() PlanV2 {
			fixture := actionFixture()
			fixture.Actions = make([]PlanAction, maxPlanActionsPerPlan+1)
			for i := range fixture.Actions {
				fixture.Actions[i] = PlanAction{Event: PlanActionOnPlanStart, Type: PlanActionCompact}
			}
			return fixture
		}(),
	}
	for name, fixture := range planCases {
		t.Run(name, func(t *testing.T) {
			_, _, err := m.ReplacePlanV2(fixture, false)
			assert.Error(t, err, "invalid plan action must fail closed")
		})
	}
}

func TestReplacePlanV2ValidatesModels(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, err := m.ReplacePlanV2(actionFixture(), false)
	require.NoError(t, err)

	cases := map[string]func(*PlanV2){
		"step model too long":     func(p *PlanV2) { p.Items[1].Model = strings.Repeat("m", maxPlanModelRunes+1) },
		"step model blank":        func(p *PlanV2) { p.Items[1].Model = " " },
		"type model too long":     func(p *PlanV2) { p.ModelsByType[StepEdit] = strings.Repeat("m", maxPlanModelRunes+1) },
		"type model blank":        func(p *PlanV2) { p.ModelsByType[StepEdit] = " " },
		"type model unknown type": func(p *PlanV2) { p.ModelsByType[StepType("ship")] = "opus" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := actionFixture()
			mutate(&fixture)
			_, _, err := m.ReplacePlanV2(fixture, false)
			assert.Error(t, err, "invalid model reference must fail closed")
		})
	}
}

func TestPatchActionsAndModelsAreMaterial(t *testing.T) {
	cases := []struct {
		name  string
		ops   []PlanPatchOp
		diff  []PlanMaterialChange
		fixed func(*testing.T, Plan)
	}{
		{
			"step model change revokes approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Model: pv("opus-pro")}},
			[]PlanMaterialChange{
				{Target: "decode-legacy", Field: "model", Change: MaterialChanged, Detail: "haiku to opus-pro"},
			},
			func(t *testing.T, plan Plan) {
				assert.Equal(t, "opus-pro", plan.Items[1].Model)
			},
		},
		{
			"step model clear revokes approval",
			[]PlanPatchOp{{Op: PlanPatchUpdateStep, ID: "decode-legacy", Model: pv("")}},
			[]PlanMaterialChange{
				{Target: "decode-legacy", Field: "model", Change: MaterialChanged, Detail: "haiku to "},
			},
			nil,
		},
		{
			"step action change revokes approval",
			[]PlanPatchOp{{
				Op: PlanPatchUpdateStep, ID: "decode-legacy",
				Actions: pvActions([]PlanAction{{Event: PlanActionOnStepEnd, Type: PlanActionCompact}}),
			}},
			[]PlanMaterialChange{
				{
					Target: "decode-legacy",
					Field:  "actions",
					Change: MaterialChanged,
					Detail: "1: step_start inject_skill",
				},
				{Target: "decode-legacy", Field: "actions", Change: MaterialRemoved, Detail: "2: step_end compact"},
			},
			nil,
		},
		{
			"type model change revokes approval",
			[]PlanPatchOp{{
				Op:           PlanPatchSetPlanFields,
				ModelsByType: pvModels(map[StepType]string{StepEdit: "opus", StepRun: "haiku"}),
			}},
			[]PlanMaterialChange{
				{Target: "plan", Field: "modelsByType", Change: MaterialChanged, Detail: "run: sonnet to haiku"},
			},
			func(t *testing.T, plan Plan) {
				assert.Equal(t, map[StepType]string{StepEdit: "opus", StepRun: "haiku"}, plan.ModelsByType)
			},
		},
		{
			"plan action change revokes approval",
			[]PlanPatchOp{{
				Op:      PlanPatchSetPlanFields,
				Actions: pvActions([]PlanAction{{Event: PlanActionOnPlanEnd, Type: PlanActionCompact}}),
			}},
			[]PlanMaterialChange{
				{Target: "plan", Field: "actions", Change: MaterialChanged, Detail: "1: plan_start compact"},
			},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
			require.NoError(t, err)
			_, _, err = m.ReplacePlanV2(actionFixture(), true)
			require.NoError(t, err)
			require.True(t, m.Plan().Approved)

			patched, summary, err := m.PatchPlan(m.Plan().Revision, tc.ops, false)
			require.NoError(t, err)
			assert.False(t, patched.Approved)
			assert.Equal(t, tc.diff, summary.Diff)
			if tc.fixed != nil {
				tc.fixed(t, patched)
			}
		})
	}
}

func TestPatchPlanRejectsInvalidAutomation(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, err := m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	ops := []PlanPatchOp{{
		Op: PlanPatchUpdateStep, ID: "decode-legacy",
		Actions: pvActions([]PlanAction{{Event: "step_middle", Type: PlanActionCompact}}),
	}}
	_, _, err = m.PatchPlan(m.Plan().Revision, ops, false)
	assert.Error(t, err, "patched actions run through the same validation throat")
}

func TestAppendPlanActionRun(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	plan, err := m.AppendPlanActionRun("decode-legacy", 0, PlanActionRun{Status: PlanActionRunOK})
	require.NoError(t, err)
	require.Len(t, plan.Items[1].Actions[0].Runs, 1)
	assert.Equal(t, PlanActionRunOK, plan.Items[1].Actions[0].Runs[0].Status)
	assert.False(t, plan.UpdatedAt.IsZero(), "the run stamps its own time")

	// Run history is operational: it must not revoke approval and must
	// survive the next patch.
	require.True(t, plan.Approved)
	patched, _, err := m.PatchPlan(plan.Revision, []PlanPatchOp{
		{Op: PlanPatchUpdateStep, ID: "decode-legacy", Why: pv("reworded")},
	}, false)
	require.NoError(t, err)
	assert.True(t, patched.Approved, "run history is operational")
	require.Len(t, patched.Items[1].Actions[0].Runs, 1)

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	assert.Len(t, loaded.Plan().Items[1].Actions[0].Runs, 1, "runs persist through the log")

	_, err = m.AppendPlanActionRun("decode-legacy", 5, PlanActionRun{Status: PlanActionRunOK})
	assert.Error(t, err, "unknown action index fails closed")
	_, err = m.AppendPlanActionRun("no-such-step", 0, PlanActionRun{Status: PlanActionRunOK})
	assert.Error(t, err, "unknown step fails closed")
}

func TestAppendPlanActionRunCapsTail(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, err := m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	for i := 0; i < maxPlanActionRunsKept+3; i++ {
		_, err := m.AppendPlanActionRun("decode-legacy", 0, PlanActionRun{Status: PlanActionRunOK})
		require.NoError(t, err)
	}
	plan := m.Plan()
	assert.Len(t, plan.Items[1].Actions[0].Runs, maxPlanActionRunsKept, "the run tail is bounded")
}

func TestAppendPlanActionRunPlanLevel(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, err := m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	plan, err := m.AppendPlanActionRun("", 0, PlanActionRun{Status: PlanActionRunFailed, Error: "compaction declined"})
	require.NoError(t, err)
	require.Len(t, plan.Actions[0].Runs, 1)
	assert.Equal(t, "compaction declined", plan.Actions[0].Runs[0].Error)

	// The plan_start action has an index; the plan-level action list is
	// addressed with the empty step id, so a bogus index must fail.
	_, err = m.AppendPlanActionRun("", 2, PlanActionRun{Status: PlanActionRunOK})
	assert.Error(t, err)
}

func TestLoadedPlanRejectsGarbageAutomation(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	plan := m.Plan()
	plan.Items[1].Actions[0].Event = "step_middle"
	// The load seam replays the log, so append a snapshot this harness never
	// wrote and expect the load path to fail closed on it.
	f, err := os.OpenFile(m.File(), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(f).Encode(PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: "garbage", Timestamp: plan.UpdatedAt},
		Plan:             plan,
	}))
	require.NoError(t, f.Close())

	_, err = OpenSession(m.File())
	assert.Error(t, err, "load fails closed on an event this harness never wrote")
}
