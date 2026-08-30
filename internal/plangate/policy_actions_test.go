package plangate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func runTypeWithActions(actions ...session.PlanAction) Defaults {
	return Defaults{Types: []TypeDefaults{{
		Name:    session.StepRun,
		Tools:   []string{"bash"},
		Actions: actions,
	}}}
}

// Default actions ride the same Compile freeze as tools and model pins: both
// scopes validate through the session normalizer and come back detached, with
// no run history — defaults define automation, they never record it.
func TestCompileFreezesDefaultActions(t *testing.T) {
	defaults := DefaultDefaults()
	defaults.Actions = []session.PlanAction{{
		Event: session.PlanActionOnPlanEnd,
		Type:  session.PlanActionCompact,
		Runs:  []session.PlanActionRun{{Status: session.PlanActionRunOK}},
	}}
	for i, typ := range defaults.Types {
		if typ.Name != session.StepRun {
			continue
		}
		defaults.Types[i].Actions = []session.PlanAction{{
			Event:  session.PlanActionOnStepStart,
			Type:   session.PlanActionInjectSkill,
			Skills: []string{"tdd"},
			Runs:   []session.PlanActionRun{{Status: session.PlanActionRunOK}},
		}}
	}

	policy, err := Compile(defaults)
	require.NoError(t, err)

	plan := policy.PlanActions()
	require.Len(t, plan, 1)
	assert.Equal(t, session.PlanAction{
		Event: session.PlanActionOnPlanEnd,
		Type:  session.PlanActionCompact,
	}, plan[0], "run history is stripped at the freeze")

	byType := policy.ActionsByType()
	require.Contains(t, byType, session.StepRun)
	require.Len(t, byType[session.StepRun], 1)
	step := byType[session.StepRun][0]
	assert.Equal(t, session.PlanActionOnStepStart, step.Event)
	assert.Equal(t, session.PlanActionInjectSkill, step.Type)
	assert.Equal(t, []string{"tdd"}, step.Skills)
	assert.Empty(t, step.Runs)
	assert.NotContains(t, byType, session.StepExplore, "types without actions stay absent")

	// Detached: mutating the caller's lists after Compile cannot reach the policy.
	defaults.Actions[0].Type = session.PlanActionInjectSkill
	defaults.Actions[0].Skills = []string{"oops"}
	assert.Equal(t, session.PlanActionCompact, policy.PlanActions()[0].Type)

	// Defaults() hands out an editable copy that carries the actions, detached
	// from the policy it came from.
	draft := policy.Defaults()
	assert.Equal(t, plan, draft.Actions)
	draft.Actions[0].Type = session.PlanActionInjectSkill
	assert.Equal(t, session.PlanActionCompact, policy.PlanActions()[0].Type)
	for _, typ := range draft.Types {
		if typ.Name == session.StepRun {
			assert.Equal(t, byType[session.StepRun], typ.Actions)
		}
	}
}

// Mis-scoped or malformed action lists fail Compile closed: a settings draft
// carrying them never reaches the runtime or the config file.
func TestCompileRejectsMisScopedDefaultActions(t *testing.T) {
	cases := map[string]Defaults{
		"step event at plan level": {Actions: []session.PlanAction{{
			Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
		}}},
		"plan event at type level": runTypeWithActions(session.PlanAction{
			Event: session.PlanActionOnPlanEnd, Type: session.PlanActionCompact,
		}),
		"inject_skill without skills": {Actions: []session.PlanAction{{
			Event: session.PlanActionOnPlanEnd, Type: session.PlanActionInjectSkill,
		}}},
		"compact with skills": runTypeWithActions(session.PlanAction{
			Event: session.PlanActionOnStepStart, Type: session.PlanActionCompact,
			Skills: []string{"tdd"},
		}),
		"unknown action type": runTypeWithActions(session.PlanAction{
			Event: session.PlanActionOnStepStart, Type: "run_tests",
		}),
		"too many actions": {Actions: []session.PlanAction{
			{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
			{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
			{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
			{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
			{Event: session.PlanActionOnPlanStart, Type: session.PlanActionCompact},
		}},
	}
	for name, defaults := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Compile(defaults)
			require.Error(t, err)
		})
	}
}
