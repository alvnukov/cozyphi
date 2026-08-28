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
	assert.False(
		t,
		policy.Check(
			plangate.PhaseDeny,
			plan,
			plangate.ToolCall{Name: "read", Step: plangate.StepRef{Ordinal: 1}},
		).Deny,
	)
	assert.False(
		t,
		policy.Check(
			plangate.PhaseDeny,
			plan,
			plangate.ToolCall{Name: "edit", Step: plangate.StepRef{Ordinal: 1}},
		).Deny,
	)
	assert.True(
		t,
		policy.Check(
			plangate.PhaseDeny,
			plan,
			plangate.ToolCall{Name: "bash", Step: plangate.StepRef{Ordinal: 1}},
		).Deny,
	)
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

	assert.True(t, checker.Check(plan, plangate.ToolCall{Name: "edit", Step: plangate.StepRef{Ordinal: 1}}).Deny)
	require.NoError(t, runtime.Apply(plangate.Defaults{Types: []plangate.TypeDefaults{{
		Name: "work", Tools: []string{"read", "edit"},
	}}}))
	assert.False(t, checker.Check(plan, plangate.ToolCall{Name: "edit", Step: plangate.StepRef{Ordinal: 1}}).Deny)
}

func TestPolicyWithNoTypesDisablesNonEmptyPlans(t *testing.T) {
	policy, err := plangate.Compile(plangate.Defaults{Types: []plangate.TypeDefaults{}})
	require.NoError(t, err)
	require.NoError(t, policy.ValidateItems(nil))
	assert.ErrorContains(t, policy.ValidateItems([]session.PlanItem{{
		Content: "cannot start", Status: session.PlanPending, Type: "anything",
	}}), "plan creation is disabled")
}

// TestFinishedPlanDischargesTheGate: a closed plan is a discharged contract —
// every call passes without naming a step, and the provider sees only the
// exempt set because nothing typed is startable on terminal steps.
func TestFinishedPlanDischargesTheGate(t *testing.T) {
	policy, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)

	closed := session.Plan{
		Approved: true,
		Result:   session.PlanResultSuccess,
		Items: []session.PlanItem{{
			ID: "wire", Content: "wire the renderer", Status: session.PlanInProgress, Type: session.StepEdit,
		}},
	}

	verdict := policy.Check(
		plangate.PhaseDeny, closed,
		plangate.ToolCall{Name: "edit", Step: plangate.StepRef{ID: "ghost"}},
	)
	assert.False(t, verdict.Miss, "a finished plan no longer gates tool calls")
	assert.False(t, verdict.Deny)

	visible := policy.VisibleTools(closed)
	assert.Contains(t, visible, "plan")
	assert.NotContains(t, visible, "edit", "nothing typed is startable on a closed plan")
}

func TestPolicyVisibleToolsMirrorsTheGate(t *testing.T) {
	policy, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)

	exempt := []string{"plan", "context", "question", "watch", "memory"}
	cases := []struct {
		name string
		plan session.Plan
		want []string
	}{
		{
			name: "unapproved plan narrows to the exempt set",
			plan: session.Plan{Items: []session.PlanItem{{
				Content: "change", Status: session.PlanInProgress, Type: session.StepEdit,
			}}},
			want: exempt,
		},
		{
			name: "pending step shows its tools so the first call is possible",
			plan: session.Plan{Approved: true, Items: []session.PlanItem{{
				Content: "later", Status: session.PlanPending, Type: session.StepRun,
			}}},
			want: []string{
				"read", "grep", "find", "ls", "lsp", "write", "edit", "bash",
				"plan", "context", "question", "watch", "memory",
			},
		},
		{
			name: "explore step shows the read-only set",
			plan: session.Plan{Approved: true, Items: []session.PlanItem{{
				Content: "look", Status: session.PlanInProgress, Type: session.StepExplore,
			}}},
			want: append([]string{"read", "grep", "find", "ls", "lsp"}, exempt...),
		},
		{
			name: "edit step inherits the read-only set",
			plan: session.Plan{Approved: true, Items: []session.PlanItem{{
				Content: "change", Status: session.PlanInProgress, Type: session.StepEdit,
			}}},
			want: []string{
				"read", "grep", "find", "ls", "lsp", "write", "edit",
				"plan", "context", "question", "watch", "memory",
			},
		},
		{
			name: "two active steps show the union",
			plan: session.Plan{Approved: true, Items: []session.PlanItem{
				{Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
				{Content: "run", Status: session.PlanInProgress, Type: session.StepRun},
			}},
			want: []string{
				"read", "grep", "find", "ls", "lsp", "write", "edit", "bash",
				"plan", "context", "question", "watch", "memory",
			},
		},
		{
			name: "an unknown step type widens nothing",
			plan: session.Plan{Approved: true, Items: []session.PlanItem{{
				Content: "mystery", Status: session.PlanInProgress, Type: session.StepType("mystery"),
			}}},
			want: exempt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visible := policy.VisibleTools(tc.plan)
			for _, name := range tc.want {
				assert.Contains(t, visible, name)
			}
			assert.Len(t, visible, len(tc.want), "the visible set must be exactly the allowed set")
		})
	}

	// The mirror property: a tool is visible exactly when the gate would let
	// it run against a step the plan offers — in_progress now, or pending and
	// startable by naming it.
	plan := session.Plan{Approved: true, Items: []session.PlanItem{{
		Content: "change", Status: session.PlanInProgress, Type: session.StepEdit,
	}}}
	pendingPlan := session.Plan{Approved: true, Items: []session.PlanItem{{
		ID: "only", Content: "change", Status: session.PlanPending, Type: session.StepEdit,
	}}}
	visible := policy.VisibleTools(plan)
	pendingVisible := policy.VisibleTools(pendingPlan)
	for _, name := range []string{"read", "edit", "bash", "agent_spawn", "mcp_call"} {
		_, seen := visible[name]
		missed := policy.Check(
			plangate.PhaseDeny,
			plan,
			plangate.ToolCall{Name: name, Step: plangate.StepRef{Ordinal: 1}},
		).Miss
		assert.Equal(t, seen, !missed, "tool %s: visibility must match the gate verdict", name)
		_, pendingSeen := pendingVisible[name]
		pendingMissed := policy.Check(
			plangate.PhaseDeny,
			pendingPlan,
			plangate.ToolCall{Name: name, Step: plangate.StepRef{ID: "only"}},
		).Miss
		assert.Equal(t, pendingSeen, !pendingMissed, "tool %s: pending visibility must match the gate verdict", name)
	}
}
