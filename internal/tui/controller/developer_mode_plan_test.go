package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planContract is the smallest v2 work contract the authoring path accepts.
func planContract(items ...session.PlanItem) session.PlanV2 {
	return session.PlanV2{
		Goal:            "wire the plan category",
		Approach:        "author it, approve it, start it",
		SuccessCriteria: []string{"the category answers"},
		Items:           items,
	}
}

func planField(t *testing.T, c *Controller, key string) diag.Field {
	t.Helper()
	explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryPlan, key)
	require.NoError(t, err)
	return explained.Field
}

// A fresh useplan session has a gate, a published policy and no plan. Those
// four facts are exactly what a model deciding what it may do next needs.
func TestTheHarnessReportsThePlanPostureThisSessionActuallyRunsUnder(t *testing.T) {
	c := developerToolRuntime(t)

	assert.True(t, planField(t, c, diag.KeyPlanEnabled).Effective.Value.Bool)
	assert.Equal(t, "useplan", planField(t, c, diag.KeyPlanMode).Effective.Value.Str)
	assert.Equal(t, "deny", planField(t, c, diag.KeyPlanGatePhase).Effective.Value.Str)
	assert.Equal(t, diag.PlanLifecycleAbsent,
		planField(t, c, diag.KeyPlanLifecycle).Effective.Value.Str)

	types := planField(t, c, diag.KeyPlanPolicyTypes)
	assert.Contains(t, types.Effective.Value.List, "explore",
		"the model is being shown a vocabulary, and this is the one")
	assert.NotEmpty(t, types.Revision, "a policy answer names the policy it read")
}

// The live wiring: the plan moves through the session that owns it, and the
// next question is answered from that session rather than from a cached view.
func TestThePlanAnswerFollowsTheSessionThroughItsLifecycle(t *testing.T) {
	c := developerToolRuntime(t)

	_, err := c.CreatePlan(t.Context(), planContract(session.PlanItem{
		ID: "look", Content: "look around", Status: session.PlanPending,
		Type: session.StepExplore, Why: "nothing is known yet", DoneWhen: "the seam is found",
	}))
	require.NoError(t, err)

	assert.Equal(t, diag.PlanLifecycleDraft,
		planField(t, c, diag.KeyPlanLifecycle).Effective.Value.Str,
		"an authored contract nobody approved is a draft")
	assert.Equal(t, int64(1), planField(t, c, diag.KeyPlanStepsTotal).Effective.Value.Int)
	assert.Equal(t, int64(1), planField(t, c, diag.KeyPlanStepsRemaining).Effective.Value.Int)

	require.NoError(t, c.SetPlanApproved(true))
	assert.Equal(t, diag.PlanLifecycleApproved,
		planField(t, c, diag.KeyPlanLifecycle).Effective.Value.Str,
		"approved with nothing started yet is not the same as running")
	assert.Equal(t, diag.StateUnset, planField(t, c, diag.KeyPlanStepCurrent).Effective.State)

	// Starting the step is operational, not contractual, so the user's
	// approval survives it — and the next answer is running, not re-drafted.
	_, err = c.CreatePlan(t.Context(), planContract(session.PlanItem{
		ID: "look", Content: "look around", Status: session.PlanInProgress,
		Type: session.StepExplore, Why: "nothing is known yet", DoneWhen: "the seam is found",
	}))
	require.NoError(t, err)

	assert.Equal(t, diag.PlanLifecycleActive,
		planField(t, c, diag.KeyPlanLifecycle).Effective.Value.Str)
	assert.Equal(t, "look", planField(t, c, diag.KeyPlanStepCurrent).Effective.Value.Str)
	assert.Equal(t, int64(0), planField(t, c, diag.KeyPlanStepAttempts).Effective.Value.Int,
		"the step has been started and nothing has been tried under it yet")
	assert.Contains(t, planField(t, c, diag.KeyPlanStepTools).Effective.Value.List, "read",
		"an approved explore step in progress admits reading")
	assert.NotContains(t, planField(t, c, diag.KeyPlanStepTools).Effective.Value.List, "bash",
		"and nothing a later step type introduces")
}

// The category names the plan and never quotes it. A plan is where the
// model's own prose lives durably, so this is the leak surface that matters.
func TestThePlanAnswerCarriesNoContractProse(t *testing.T) {
	c := developerToolRuntime(t)

	_, err := c.CreatePlan(t.Context(), session.PlanV2{
		Goal:            "SENTINEL-GOAL",
		Approach:        "SENTINEL-APPROACH",
		WorkingContext:  "SENTINEL-CONTEXT",
		SuccessCriteria: []string{"SENTINEL-CRITERION"},
		Constraints:     []string{"SENTINEL-CONSTRAINT"},
		Items: []session.PlanItem{{
			ID: "look", Content: "SENTINEL-CONTENT", Status: session.PlanInProgress,
			Type: session.StepExplore, Why: "SENTINEL-WHY", DoneWhen: "SENTINEL-DONE",
			Risk: "SENTINEL-RISK", Note: "SENTINEL-NOTE",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, c.SetPlanApproved(true))

	snapshot, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPlan)
	require.NoError(t, err)
	rendered, err := json.Marshal(snapshot)
	require.NoError(t, err)

	body := string(rendered)
	for _, forbidden := range []string{
		"SENTINEL-GOAL", "SENTINEL-APPROACH", "SENTINEL-CONTEXT", "SENTINEL-CRITERION",
		"SENTINEL-CONSTRAINT", "SENTINEL-CONTENT", "SENTINEL-WHY", "SENTINEL-DONE",
		"SENTINEL-RISK", "SENTINEL-NOTE",
	} {
		assert.NotContains(t, body, forbidden,
			"the plan's prose belongs to the session, not to a diagnostics answer")
	}
	assert.Contains(t, body, "look", "the step id is the identity a plan_step names")
}

// Asking is not authoring: no revision moves, no step starts, no counter
// counts a question as work the harness did.
func TestObservingThePlanChangesNothingAboutIt(t *testing.T) {
	c := developerToolRuntime(t)
	engine := c.engineRef.Load()
	before := engine.PlanObservation()

	for range 3 {
		planField(t, c, diag.KeyPlanLifecycle)
		_, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPlan)
		require.NoError(t, err)
	}

	assert.Equal(t, before, engine.PlanObservation(),
		"the whole observation is the same one, down to the counters")
}

// The plan category is the whole of a detail answer and one claim on a shared
// overview budget. It has to fit the first; the second it divides with ten
// other categories, and when its share runs out the answer says so rather
// than quietly answering less.
func TestThePlanCategoryFitsItsOwnAnswerAndRidesTheOverview(t *testing.T) {
	c := developerToolRuntime(t)

	detail, err := c.diagnostics.Snapshot(t.Context(), diag.CategoryPlan)
	require.NoError(t, err)
	require.Len(t, detail.Categories, 1)
	assert.False(t, detail.Truncated, "a category asked for by name answers in full")
	assert.Equal(t, diag.AvailabilityAvailable, detail.Categories[0].Availability)
	require.NotEmpty(t, detail.Categories[0].Fields)

	overview, err := c.diagnostics.Snapshot(t.Context(), "")
	require.NoError(t, err)

	plan := overviewOf(t, overview, diag.CategoryPlan)
	assert.NotEmpty(t, plan.Overview,
		"plan has the most to say of any category and still gets a share")
	assert.LessOrEqual(t, len(plan.Overview), len(detail.Categories[0].Fields),
		"the overview names fields the detail view explains, and invents none")
	if plan.Truncated {
		assert.True(t, overview.Truncated, "a category cut short makes the whole answer say so")
	} else {
		assert.Len(t, plan.Overview, len(detail.Categories[0].Fields),
			"a share that was enough names every field the detail view explains")
	}
	assert.Equal(t, overview.Truncated, strings.Contains(overview.Note, "size limit"),
		"a shared budget that ran out is stated, never silently applied")
}

func TestNoEngineYetLeavesThePlanCategoryUnavailable(t *testing.T) {
	c := &Controller{}
	registry := diag.NewRegistry(nil, diag.DefaultLimits(),
		diag.NewPlanCollector(diag.PlanDeps{State: c.planState}))

	explained, err := registry.Explain(t.Context(), diag.CategoryPlan, diag.KeyPlanLifecycle)
	require.NoError(t, err)
	assert.Equal(t, diag.StateUnavailable, explained.Field.Effective.State)
}
