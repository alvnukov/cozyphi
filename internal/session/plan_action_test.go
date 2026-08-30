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
			fixture.Actions = make([]PlanAction, maxPlanActions+1)
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

func TestReplacePlanV2NormalizesDisabledSkills(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	fixture := actionFixture()
	fixture.Items[1].Actions[0].DisabledSkills = []string{" code-review ", "missing", "code-review"}
	created, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"code-review"}, created.Items[1].Actions[0].DisabledSkills,
		"off marks trim, dedup, and drop names the action no longer lists")

	loaded, err := OpenSession(m.File())
	require.NoError(t, err)
	assert.Equal(t, []string{"code-review"}, loaded.Plan().Items[1].Actions[0].DisabledSkills,
		"reloading must be stable")

	compact := actionFixture()
	compact.Items[1].Actions[1].DisabledSkills = []string{"tdd"}
	_, _, err = m.ReplacePlanV2(compact, false)
	assert.Error(t, err, "compact takes no disabled skills")
}

func TestToggleDisabledSkillIsMaterialButKeepsHistory(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	plan, err := m.AppendPlanActionRun("decode-legacy", 0, PlanActionRun{Status: PlanActionRunOK})
	require.NoError(t, err)
	require.True(t, plan.Approved)

	toggled, err := m.SetPlanSkillDisabled("decode-legacy", 0, "code-review", true)
	require.NoError(t, err)
	assert.False(t, toggled.Approved, "a skill toggle changes what runs and revokes approval")
	assert.Equal(t, []PlanMaterialChange{{
		Target: "decode-legacy", Field: "actions", Change: MaterialChanged,
		Detail: "1: step_start inject_skill (off: none to code-review)",
	}}, MaterialDiff(plan, toggled))
	assert.Equal(t, []string{"tdd"}, toggled.Items[1].Actions[0].EffectiveSkills())
	assert.Len(t, toggled.Items[1].Actions[0].Runs, 1,
		"the toggle touches only the off mark, so run history must survive")

	reenabled, err := m.SetPlanSkillDisabled("decode-legacy", 0, "code-review", false)
	require.NoError(t, err)
	assert.Empty(t, reenabled.Items[1].Actions[0].DisabledSkills)
	assert.Len(t, reenabled.Items[1].Actions[0].Runs, 1, "re-enabling keeps history too")
	assert.Equal(t, []string{"tdd", "code-review"}, reenabled.Items[1].Actions[0].EffectiveSkills())

	_, err = m.SetPlanSkillDisabled("decode-legacy", 0, "missing", true)
	assert.Error(t, err, "a skill the action does not list fails closed")
	_, err = m.SetPlanSkillDisabled("decode-legacy", 1, "tdd", true)
	assert.Error(t, err, "compact carries no skills")
	_, err = m.SetPlanSkillDisabled("decode-legacy", 9, "tdd", true)
	assert.Error(t, err, "unknown action index fails closed")
	_, err = m.SetPlanSkillDisabled("no-such-step", 0, "tdd", true)
	assert.Error(t, err, "unknown step fails closed")
}

func TestEffectiveSkillsKeepsListedOrder(t *testing.T) {
	action := PlanAction{
		Event:          PlanActionOnStepStart,
		Type:           PlanActionInjectSkill,
		Skills:         []string{"tdd", "grill", "code-review"},
		DisabledSkills: []string{"grill"},
	}
	assert.Equal(t, []string{"tdd", "code-review"}, action.EffectiveSkills())
}

func TestNormalizeStepDefaultActionsClonesDisabled(t *testing.T) {
	defaults := []PlanAction{{
		Event:          PlanActionOnStepStart,
		Type:           PlanActionInjectSkill,
		Skills:         []string{"tdd"},
		DisabledSkills: []string{"tdd"},
	}}
	frozen, err := NormalizeStepDefaultActions(defaults)
	require.NoError(t, err)
	frozen[0].DisabledSkills[0] = "mutated"
	assert.Equal(t, []string{"tdd"}, defaults[0].DisabledSkills,
		"frozen defaults must not alias the authored slice")
}

// TestAuthoredStepSkillsCompileToInjectAction: the wire skills list is the
// author's say; Actions stays the one canonical home. An authored list
// displaces whatever the type defaults seeded — including the seeded skill
// names — the empty list removes the injection outright, off marks survive
// only for names the new list still carries, and the input field never
// persists.
func TestAuthoredStepSkillsCompileToInjectAction(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)

	seeded := []PlanAction{
		{
			Event:          PlanActionOnStepStart,
			Type:           PlanActionInjectSkill,
			Skills:         []string{"tdd", "grill"},
			DisabledSkills: []string{"grill"},
		},
		{Event: PlanActionOnStepEnd, Type: PlanActionCompact},
	}

	f := actionFixture()
	f.Items[1].Skills = []string{"tdd", "code-review"}
	f.Items[1].Actions = ClonePlanActions(seeded)
	plan, _, err := m.ReplacePlanV2(f, true)
	require.NoError(t, err)
	stored := plan.Items[1]
	assert.Nil(t, stored.Skills, "the input field compiles away and never persists")
	require.Len(t, stored.Actions, 2)
	assert.Equal(t, []string{"tdd", "code-review"}, stored.Actions[0].Skills,
		"the authored list displaces the seeded one")
	assert.Empty(t, stored.Actions[0].DisabledSkills,
		"an off mark dies with the name that left the list")
	assert.Equal(t, PlanActionCompact, stored.Actions[1].Type, "other automation survives")

	f2 := actionFixture()
	f2.Items[1].Skills = []string{"tdd"}
	f2.Items[1].Actions = nil
	plan2, _, err := m.ReplacePlanV2(f2, true)
	require.NoError(t, err)
	require.Len(t, plan2.Items[1].Actions, 1, "no prior action: the injection is appended")
	assert.Equal(t, PlanActionInjectSkill, plan2.Items[1].Actions[0].Type)
	assert.Equal(t, []string{"tdd"}, plan2.Items[1].Actions[0].Skills)

	f3 := actionFixture()
	f3.Items[1].Skills = []string{}
	f3.Items[1].Actions = ClonePlanActions(seeded)
	plan3, _, err := m.ReplacePlanV2(f3, true)
	require.NoError(t, err)
	require.Len(t, plan3.Items[1].Actions, 1, "explicit empty list removes the injection")
	assert.Equal(t, PlanActionCompact, plan3.Items[1].Actions[0].Type)

	f4 := actionFixture()
	f4.Items[1].Actions = ClonePlanActions(seeded)
	plan4, _, err := m.ReplacePlanV2(f4, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"tdd", "grill"}, plan4.Items[1].Actions[0].Skills,
		"absent field: the seeded list stands")
	assert.Equal(t, []string{"grill"}, plan4.Items[1].Actions[0].DisabledSkills)
}

// TestPatchUpdateStepSkillsReplacesInjection: update_step.skills is the
// model's narrow path into step automation — set replaces the injected list
// (the empty value removes the injection), unset leaves it alone.
func TestPatchUpdateStepSkillsReplacesInjection(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, _, err = m.ReplacePlanV2(actionFixture(), true)
	require.NoError(t, err)

	plan, _, err := m.PatchPlan(m.Plan().Revision, []PlanPatchOp{{
		Op: PlanPatchUpdateStep, ID: "decode-legacy",
		Skills: PatchValue[[]string]{Set: true, Value: []string{"tdd", "grill"}},
	}}, false)
	require.NoError(t, err)
	require.Len(t, plan.Items[1].Actions, 2)
	assert.Equal(t, []string{"tdd", "grill"}, plan.Items[1].Actions[0].Skills,
		"the set value replaces the injected list")

	plan2, _, err := m.PatchPlan(plan.Revision, []PlanPatchOp{{
		Op: PlanPatchUpdateStep, ID: "decode-legacy",
		Skills: PatchValue[[]string]{Set: true},
	}}, false)
	require.NoError(t, err)
	require.Len(t, plan2.Items[1].Actions, 1, "the empty value removes the injection")
	assert.Equal(t, PlanActionCompact, plan2.Items[1].Actions[0].Type)

	_, summary, err := m.PatchPlan(plan2.Revision, []PlanPatchOp{{
		Op: PlanPatchUpdateStep, ID: "decode-legacy",
		Note: PatchValue[string]{Set: true, Value: "skills untouched"},
	}}, false)
	require.NoError(t, err)
	assert.Empty(t, summary.Diff, "an unset skills slot changes nothing material")
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

	for range maxPlanActionRunsKept + 3 {
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
