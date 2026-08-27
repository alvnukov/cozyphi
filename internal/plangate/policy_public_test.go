package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestPolicyCompilesCustomHierarchyAndValidatesPlans(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{
		Types: []plangate.TypeDefaults{
			{Name: "inspect", Tools: []string{"read", "grep"}},
			{Name: "change", Tools: []string{"write", "edit"}},
			{Name: "execute", Tools: []string{"bash"}},
		},
		AdditionalExemptions: []string{"lsp"},
	})
	require.NoError(t, err)

	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		Content: "change files", Status: session.PlanInProgress, Type: "change",
	}}}
	assert.False(t, policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "read", PlanStep: 1}).Deny)
	assert.False(t, policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "edit", PlanStep: 1}).Deny)
	assert.True(t, policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "bash", PlanStep: 1}).Deny)
	assert.False(t, policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "lsp"}).Deny,
		"additional exemptions always bypass the plan gate")
	assert.False(t, policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "plan"}).Deny,
		"mandatory exemptions cannot be removed")

	require.NoError(t, policy.ValidateItems(plan.Items))
	assert.ErrorContains(
		t,
		policy.ValidateItems([]session.PlanItem{{Content: "missing type", Status: session.PlanPending}}),
		"type is required",
	)
	assert.ErrorContains(t, policy.ValidateItems([]session.PlanItem{{
		Content: "unknown type", Status: session.PlanPending, Type: "ghost",
	}}), "unknown step type")
}

func TestPolicyProjectsConfiguredTypesIntoPromptAndSchemaInputs(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{
		{Name: "inspect", Tools: []string{"read"}},
		{Name: "change", Tools: []string{"edit"}},
	}})
	require.NoError(t, err)

	assert.Equal(t, []string{"inspect", "change"}, policy.StepTypes())
	prompt := policy.PromptBlock(plangate.PhaseDeny)
	assert.Contains(t, prompt, "- inspect: read")
	assert.Contains(t, prompt, "- change: read, edit")
	assert.NotContains(t, prompt, "Steps may omit their type")
}

func TestRuntimePublishesPolicyForTheNextCheck(t *testing.T) {
	runtime, err := plangate.NewRuntime(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "work", Tools: []string{"read"},
	}}})
	require.NoError(t, err)
	checker := plangate.Checker{Phase: plangate.PhaseDeny, Runtime: runtime}
	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		Content: "work", Status: session.PlanInProgress, Type: "work",
	}}}

	assert.True(t, checker.Check(plan, plangate.ToolCall{Name: "edit", PlanStep: 1}).Deny)
	require.NoError(t, runtime.Apply(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "work", Tools: []string{"read", "edit"},
	}}}))
	assert.False(t, checker.Check(plan, plangate.ToolCall{Name: "edit", PlanStep: 1}).Deny)
}

func TestPolicyWithNoTypesDisablesNonEmptyPlans(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{}})
	require.NoError(t, err)
	require.NoError(t, policy.ValidateItems(nil))
	assert.ErrorContains(t, policy.ValidateItems([]session.PlanItem{{
		Content: "cannot start", Status: session.PlanPending, Type: "anything",
	}}), "plan creation is disabled")
}
