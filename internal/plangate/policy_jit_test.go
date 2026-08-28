package plangate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// jitPlan builds an approved plan whose single run step is marked
// just-in-time with a declared irreversible risk.
func jitPlan(status session.PlanStatus) session.Plan {
	return approved(session.PlanItem{
		ID:      "push-tag",
		Content: "push the release tag",
		Status:  status,
		Type:    session.StepRun,
		Risk:    "a published tag is irreversible",
		JIT:     true,
	})
}

// granted stamps a matching user grant at the plan's contract epoch.
func granted(plan session.Plan, stepID string, epoch uint64) session.Plan {
	plan.ContractEpoch = epoch
	plan.JITApprovals = []session.JITApproval{{StepID: stepID, Epoch: epoch}}
	return plan
}

func TestCheckJITStepWithoutGrantDemandsApproval(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(jitPlan(session.PlanInProgress), ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}})

	require.NotNil(t, v.JIT, "an ungranted just-in-time step demands user approval")
	assert.False(t, v.Miss, "a just-in-time demand is a user handoff, not a model misstep")
	assert.False(t, v.Deny)
	assert.Equal(t, "push-tag", v.JIT.StepID)
	assert.Equal(t, "push the release tag", v.JIT.Action)
	assert.Equal(t, "a published tag is irreversible", v.JIT.Risk)
	assert.Equal(t, "push-tag", v.StepID, "the call still names the step it advances")
	assert.False(t, v.StartPending)
}

func TestCheckJITStepWithGrantPasses(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(granted(jitPlan(session.PlanInProgress), "push-tag", 1), ToolCall{
		Name: "bash", Step: StepRef{ID: "push-tag"},
	})
	assert.Nil(t, v.JIT, "a grant at the current epoch satisfies the demand")
	assert.False(t, v.Miss)
	assert.Equal(t, "push-tag", v.StepID)
}

func TestCheckJITGrantStaleAfterMaterialChange(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	plan := granted(jitPlan(session.PlanInProgress), "push-tag", 1)
	plan.ContractEpoch = 2 // the contract moved after the grant
	v := c.Check(plan, ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}})
	require.NotNil(t, v.JIT, "a grant dies with the contract epoch it was pinned to")
}

func TestCheckJITGrantDoesNotCoverOtherStep(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	plan := approved(
		session.PlanItem{
			ID: "push-tag", Content: "push the release tag", Status: session.PlanInProgress,
			Type: session.StepRun, Risk: "a published tag is irreversible", JIT: true,
		},
		session.PlanItem{
			ID: "notify-users", Content: "send the announcement", Status: session.PlanPending,
			Type: session.StepIntegrate, Risk: "sent mail cannot be unsent", JIT: true,
		},
	)
	plan = granted(plan, "notify-users", 1)
	v := c.Check(plan, ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}})
	require.NotNil(t, v.JIT, "a grant never crosses steps")
	assert.Equal(t, "push-tag", v.JIT.StepID)
}

func TestCheckNonJITStepNeverDemands(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	plan := jitPlan(session.PlanInProgress)
	plan.Items[0].JIT = false
	v := c.Check(plan, ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}})
	assert.Nil(t, v.JIT, "a step without the marker never asks")
	assert.False(t, v.Miss)
}

func TestCheckJITPendingStepDemandsBeforeStart(t *testing.T) {
	c := Checker{Phase: PhaseDeny}
	v := c.Check(jitPlan(session.PlanPending), ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}})
	require.NotNil(t, v.JIT)
	assert.True(t, v.StartPending, "the step starts only after the user approves it")
}

func TestCheckJITDemandAsksInBothPhases(t *testing.T) {
	call := ToolCall{Name: "bash", Step: StepRef{ID: "push-tag"}}
	hint := Checker{Phase: PhaseHint}
	v := hint.Check(jitPlan(session.PlanInProgress), call)
	require.NotNil(t, v.JIT, "the just-in-time handoff does not depend on the miss phase")
}

// TestCheckJITDemandsAcrossIrreversibleVerbs walks the representative
// irreversible effects the contract names: every one of them demands the
// user's own approval with its action and risk carried through.
func TestCheckJITDemandsAcrossIrreversibleVerbs(t *testing.T) {
	verbs := []struct{ id, action, risk string }{
		{"push-commits", "push commits to origin", "pushed history is public"},
		{"tag-release", "tag the release", "a published tag is irreversible"},
		{"deploy-prod", "deploy to production", "deployed state reaches users"},
		{"delete-table", "drop the staging table", "dropped data is gone"},
		{"send-mail", "send the announcement", "sent mail cannot be unsent"},
	}
	items := make([]session.PlanItem, 0, len(verbs))
	for i, verb := range verbs {
		status := session.PlanPending
		if i == 0 {
			status = session.PlanInProgress
		}
		items = append(items, session.PlanItem{
			ID: verb.id, Content: verb.action, Status: status,
			Type: session.StepRun, Risk: verb.risk, JIT: true,
		})
	}
	c := Checker{Phase: PhaseDeny}
	plan := approved(items...)
	for _, verb := range verbs {
		t.Run(verb.id, func(t *testing.T) {
			v := c.Check(plan, ToolCall{Name: "bash", Step: StepRef{ID: verb.id}})
			require.NotNil(t, v.JIT)
			assert.Equal(t, verb.action, v.JIT.Action)
			assert.Equal(t, verb.risk, v.JIT.Risk)
		})
	}
}

func TestJITDemandQuestionCarriesHumanDiff(t *testing.T) {
	q := JITDemand{
		StepID: "push-tag", Action: "push the release tag", Risk: "a published tag is irreversible",
	}.Question()
	assert.Contains(t, q, "push-tag")
	assert.Contains(t, q, "push the release tag")
	assert.Contains(t, q, "a published tag is irreversible")
}

func TestJITDemandRejectedNamesStepActionRisk(t *testing.T) {
	d := JITDemand{StepID: "push-tag", Action: "push the release tag", Risk: "a published tag is irreversible"}
	reason := d.Rejected("not yet")
	assert.Contains(t, reason, "push-tag")
	assert.Contains(t, reason, "push the release tag")
	assert.Contains(t, reason, "a published tag is irreversible")
	assert.Contains(t, reason, "not yet")

	bare := JITDemand{StepID: "push-tag", Action: "push the release tag"}.Rejected("")
	assert.Contains(t, bare, "none declared", "a missing risk is named, not silently dropped")
}

func TestPromptBlockExplainsJustInTimeSteps(t *testing.T) {
	assert.Contains(t, PromptBlock(PhaseDeny), "just-in-time")
	assert.Contains(t, PromptBlock(PhaseDeny), "jit", "the rule names the marker the model authors")
}
