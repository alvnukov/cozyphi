package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Dropping watch from the exemptions closed the plan-less hole, but a tool
// that is neither exempt nor rankable is not gated — it is unreachable. With
// the shipped defaults watch has to run inside an approved plan on a step
// that already permits shell.
func TestWatchRunsInsideAnApprovedPlanUnderShippedDefaults(t *testing.T) {
	policy, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)
	require.NotContains(t, policy.ExemptTools(), "watch", "watch is still gated")

	approved := func(typ session.StepType) session.Plan {
		return session.Plan{Approved: true, Items: []session.PlanItem{{
			ID: "s1", Content: "tail the build", Status: session.PlanInProgress, Type: typ,
		}}}
	}
	call := plangate.ToolCall{Name: "watch", Step: plangate.StepRef{ID: "s1"}}

	for _, typ := range []session.StepType{
		session.StepRun, session.StepDelegate, session.StepIntegrate,
	} {
		verdict := policy.Check(plangate.PhaseDeny, approved(typ), call)
		assert.False(t, verdict.Miss, "%s step runs watch: %s", typ, verdict.Reason)
	}

	explore := policy.Check(plangate.PhaseDeny, approved(session.StepExplore), call)
	assert.True(t, explore.Miss, "watch stays above the explore rank")

	outside := policy.Check(plangate.PhaseDeny, session.Plan{}, plangate.ToolCall{Name: "watch"})
	assert.True(t, outside.Miss, "with no plan watch is still refused — the 2026-09-14 hole stays shut")
}

// A step type can only hold a tool the capability ladder ranks, so a config
// that moves watch to another type has to compile. Before watch had a rank
// the gate answered "unknown tool" and no configuration could reach it.
func TestWatchIsAssignableToAStepType(t *testing.T) {
	defaults := plangate.DefaultDefaults()
	defaults.Types = []plangate.TypeDefaults{
		{Name: session.StepExplore, Tools: []string{"read", "watch"}},
		{Name: session.StepRun, Tools: []string{"bash"}},
	}
	policy, err := plangate.Compile(defaults)
	require.NoError(t, err, "watch is assignable")

	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		ID: "s1", Content: "watch the log", Status: session.PlanInProgress, Type: session.StepExplore,
	}}}
	verdict := policy.Check(plangate.PhaseDeny, plan,
		plangate.ToolCall{Name: "watch", Step: plangate.StepRef{ID: "s1"}})
	assert.False(t, verdict.Miss, "the configured step runs it: %s", verdict.Reason)
}

// The settings pane offers a checkbox per catalog entry, so the catalog has
// to say which names a step type may hold. An exemption-only tool has no
// rank: offered as a step-type row, one click trades its exemption for a
// draft that Compile rejects.
func TestExemptionOnlyToolsCannotEnterAStepType(t *testing.T) {
	var checked int
	for _, tool := range plangate.KnownTools() {
		if !tool.ExemptionOnly {
			continue
		}
		checked++
		_, err := plangate.Compile(plangate.Defaults{
			Types: []plangate.TypeDefaults{{Name: session.StepExplore, Tools: []string{tool.Name}}},
		})
		assert.ErrorContains(t, err, tool.Name, "%s carries no capability rank", tool.Name)
	}
	assert.Positive(t, checked, "the catalog still has exemption-only tools to guard")

	for _, tool := range plangate.KnownTools() {
		if tool.ExemptionOnly || tool.MandatoryExemption {
			continue
		}
		_, err := plangate.Compile(plangate.Defaults{
			Types: []plangate.TypeDefaults{{Name: session.StepExplore, Tools: []string{tool.Name}}},
		})
		assert.NoError(t, err, "every editable catalog entry is assignable")
	}
}
