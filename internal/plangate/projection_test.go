package plangate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// projectionFixture is a small approved v2 plan touching every projection
// slot: collapsed history, a full active step with citable attempt evidence,
// a blocked step, and one nearest pending step.
func projectionFixture() session.Plan {
	return session.Plan{
		Revision:        9,
		Approved:        true,
		Schema:          session.PlanSchemaV2,
		Goal:            "ship the bounded projection",
		Approach:        "one renderer behind snapshot and get",
		SuccessCriteria: []string{"fits the budget", "no audit in context"},
		Constraints:     []string{"no schema drift", "gate untouched"},
		WorkingContext:  "worktree plan-v2-compact-prompt-projection",
		Items: []session.PlanItem{
			{
				ID:       "audit",
				Content:  "audit the seam",
				Status:   session.PlanCompleted,
				Type:     session.StepExplore,
				Outcome:  "seam mapped",
				Why:      "old why must vanish",
				Evidence: "raw evidence log",
				Attempts: []session.PlanAttempt{
					{CallID: "old-1", Tool: "read", Status: session.AttemptSuccess, Summary: "old raw output"},
				},
			},
			{
				ID: "wire", Content: "wire the renderer", Status: session.PlanInProgress, Type: session.StepEdit,
				Why: "one authority", DoneWhen: "tests green", Risk: "renderer drift",
				Attempts: []session.PlanAttempt{{
					CallID: "toolu_1", Tool: "read", Status: session.AttemptSuccess, Summary: "saw the file",
				}},
			},
			{
				ID: "wait", Content: "await review", Status: session.PlanBlocked, Type: session.StepDelegate,
				Blocker: "reviewer offline", ResumeWhen: "review returns",
			},
			{
				ID: "migrate", Content: "migrate callers", Status: session.PlanPending, Type: session.StepEdit,
				Why: "next why", DoneWhen: "suite green",
			},
		},
	}
}

// TestProjectCarriesDecisionCoreWithoutAudit pins the short projection: the
// whole header contract, progress, the active and blocked steps in full,
// collapsed completed outcomes, brief nearest steps — and none of the audit
// trail, raw tool output, or completed-step prose.
func TestProjectCarriesDecisionCoreWithoutAudit(t *testing.T) {
	body, err := json.Marshal(Project(projectionFixture()))
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"revision": 9,
		"approved": true,
		"progress": {"total": 4, "done": 1, "active": 1, "blocked": 1, "pending": 1},
		"goal": "ship the bounded projection",
		"approach": "one renderer behind snapshot and get",
		"successCriteria": ["fits the budget", "no audit in context"],
		"constraints": ["no schema drift", "gate untouched"],
		"workingContext": "worktree plan-v2-compact-prompt-projection",
		"active": {
			"id": "wire",
			"content": "wire the renderer",
			"status": "in_progress",
			"type": "edit",
			"why": "one authority",
			"doneWhen": "tests green",
			"risk": "renderer drift",
			"attempts": [{"callId": "toolu_1", "tool": "read", "status": "success", "summary": "saw the file"}]
		},
		"blocked": [{
			"id": "wait",
			"content": "await review",
			"status": "blocked",
			"type": "delegate",
			"blocker": "reviewer offline",
			"resumeWhen": "review returns"
		}],
		"completed": [{"id": "audit", "status": "completed", "outcome": "seam mapped"}],
		"next": [{"id": "migrate", "content": "migrate callers", "status": "pending", "type": "edit"}]
	}`, string(body))

	for _, banned := range []string{
		"old raw output", "raw evidence log", "old why must vanish", "old-1", "next why", "suite green",
		"events", "mutations",
	} {
		assert.NotContains(t, string(body), banned, "audit and non-decision prose must stay in get full")
	}
}

// TestProjectKeepsExtraActiveStepsVisible: a second in_progress step (only
// craftable by the legacy update path) is never silently dropped — it rides
// the nearest-steps list carrying its own status.
func TestProjectKeepsExtraActiveStepsVisible(t *testing.T) {
	plan := session.Plan{Items: []session.PlanItem{
		{Content: "first", Status: session.PlanInProgress},
		{Content: "second", Status: session.PlanInProgress},
	}}
	proj := Project(plan)
	require.NotNil(t, proj.Active)
	assert.Equal(t, "first", proj.Active.Content)
	require.Len(t, proj.Next, 1)
	assert.Equal(t, "second", proj.Next[0].Content)
	assert.Equal(t, string(session.PlanInProgress), proj.Next[0].Status)
}

// TestProjectServesLegacyPlans: plans without ids or the v2 contract still
// project — completed steps collapse to content+status, the active step keeps
// its content, type, and note.
func TestProjectServesLegacyPlans(t *testing.T) {
	plan := session.Plan{Revision: 4, Items: []session.PlanItem{
		{Content: "inspect the seam", Status: session.PlanCompleted, Type: session.StepExplore},
		{Content: "widen the contract", Status: session.PlanInProgress, Type: session.StepEdit, Note: "one truth"},
	}}
	body, err := json.Marshal(Project(plan))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"revision": 4,
		"approved": false,
		"progress": {"total": 2, "done": 1, "active": 1},
		"active": {"content": "widen the contract", "status": "in_progress", "type": "edit", "note": "one truth"},
		"completed": [{"content": "inspect the seam", "status": "completed"}]
	}`, string(body))
}

// TestProjectEmptyPlanOmitsEmptySlots: with no plan there is nothing to
// project — revision and approval only, no zero-valued noise.
func TestProjectEmptyPlanOmitsEmptySlots(t *testing.T) {
	body, err := json.Marshal(Project(session.Plan{}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"revision": 0, "approved": false}`, string(body))
}

// maxedPlan builds a plan with every prose field at its durable cap: half the
// steps completed with outcomes, one active step carrying four attempts, the
// rest pending.
func maxedPlan(steps int) session.Plan {
	prose := strings.Repeat("x", 256)
	plan := session.Plan{
		Revision: 99, Approved: true, Schema: session.PlanSchemaV2,
		Goal:           strings.Repeat("g", 512),
		Approach:       strings.Repeat("a", 1024),
		WorkingContext: strings.Repeat("w", 2048),
	}
	for range 8 {
		plan.SuccessCriteria = append(plan.SuccessCriteria, prose)
		plan.Constraints = append(plan.Constraints, prose)
	}
	completed := steps / 2
	for i := range completed {
		plan.Items = append(plan.Items, session.PlanItem{
			ID: fmt.Sprintf("done-%d", i+1), Content: prose, Status: session.PlanCompleted,
			Type: session.StepEdit, Outcome: prose,
		})
	}
	if steps > 0 {
		active := session.PlanItem{
			ID: "live", Content: prose, Status: session.PlanInProgress, Type: session.StepEdit,
			Why: prose, DoneWhen: prose, Risk: prose,
		}
		for i := range 4 {
			active.Attempts = append(active.Attempts, session.PlanAttempt{
				CallID: fmt.Sprintf("call-%d", i+1), Tool: "bash",
				Status: session.AttemptSuccess, Summary: prose,
			})
		}
		plan.Items = append(plan.Items, active)
	}
	for i := range steps - completed - 1 {
		plan.Items = append(plan.Items, session.PlanItem{
			ID: fmt.Sprintf("next-%d", i+1), Content: prose, Status: session.PlanPending,
			Type: session.StepEdit, Why: prose, DoneWhen: prose,
		})
	}
	return plan
}

// TestProjectionByteUpperBoundAcrossSizes pins the one budget from 1 to 32
// maximal steps: the body stays inside maxProjectionBytes.
func TestProjectionByteUpperBoundAcrossSizes(t *testing.T) {
	for _, steps := range []int{1, 8, 32} {
		body, err := json.Marshal(Project(maxedPlan(steps)))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(body), maxProjectionBytes, "%d steps", steps)
		t.Logf("%d maximal steps: body %d bytes", steps, len(body))
	}
}

// TestProjectCutsHistoryBeforeConstraintsAndActive pins the documented
// truncation priority on a maximal plan: the active step's why/done_when and
// at least one citable attempt survive, and constraints lose entries only
// after completed history has already collapsed to a count.
func TestProjectCutsHistoryBeforeConstraintsAndActive(t *testing.T) {
	proj := Project(maxedPlan(32))

	require.NotNil(t, proj.Active)
	assert.NotEmpty(t, proj.Active.Why, "active why is never on the ladder")
	assert.NotEmpty(t, proj.Active.DoneWhen, "active done_when is never on the ladder")
	require.NotEmpty(t, proj.Active.Attempts, "the newest successful attempt stays citable")

	if proj.Elided != nil && proj.Elided.Constraints > 0 {
		assert.Empty(t, proj.Completed, "constraints may shrink only after completed history is count-only")
		assert.Equal(t, 16, proj.Elided.Completed, "all sixteen completed outcomes counted, not carried")
	}
}

// TestProjectBlockedWindowCounts: blocked steps beyond the window are
// counted, not carried.
func TestProjectBlockedWindowCounts(t *testing.T) {
	plan := projectionFixture()
	for i := range 6 {
		plan.Items = append(plan.Items, session.PlanItem{
			ID: fmt.Sprintf("wait-%d", i+1), Content: "waiting", Status: session.PlanBlocked,
			Type: session.StepDelegate, Blocker: "reviewer offline",
		})
	}
	proj := Project(plan)
	require.NotNil(t, proj.Elided)
	assert.Equal(t, 3, proj.Elided.Blocked, "seven blocked steps, window of four")
	assert.Len(t, proj.Blocked, 4)
}

// TestProjectionBoundSurvivesWideRunesAndBlockedSteps pins the budget as a
// hard invariant: durable prose caps are runes, so four-byte runes quadruple
// them into bytes, and blocked prose sits beyond the ordinary ladder's reach.
// This is the shape that overran the budget before the escape pass existed.
func TestProjectionBoundSurvivesWideRunesAndBlockedSteps(t *testing.T) {
	wide := strings.Repeat("🌍", 256)
	plan := maxedPlan(32)
	plan.Goal = strings.Repeat("🌍", 512)
	plan.Approach = strings.Repeat("🌍", 1024)
	plan.WorkingContext = strings.Repeat("🌍", 2048)
	for i := range plan.SuccessCriteria {
		plan.SuccessCriteria[i] = wide
	}
	for i := range plan.Constraints {
		plan.Constraints[i] = wide
	}
	for i := range plan.Items {
		plan.Items[i].Why = wide
		plan.Items[i].DoneWhen = wide
	}
	for i := range 6 {
		plan.Items = append(plan.Items, session.PlanItem{
			ID: fmt.Sprintf("wait-%d", i+1), Content: wide, Status: session.PlanBlocked,
			Type: session.StepDelegate, Why: wide, DoneWhen: wide, Risk: wide, Note: wide,
			Blocker: wide, ResumeWhen: wide,
		})
	}

	proj := Project(plan)
	body, err := json.Marshal(proj)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(body), maxProjectionBytes, "the escape pass must land the wide plan inside the budget")

	require.NotNil(t, proj.Elided)
	assert.True(t, proj.Elided.Escaped, "this plan needs the byte-floor escape")
	require.NotNil(t, proj.Active)
	assert.NotEmpty(t, proj.Active.Why)
	assert.NotEmpty(t, proj.Active.DoneWhen)
	require.NotEmpty(t, proj.Blocked, "one blocked step stays carried")
	assert.NotEmpty(t, proj.Blocked[0].Blocker, "the blocker itself survives, floored")
}

// TestProjectCountsElidedWork pins the ladder's arithmetic on a maximal
// plan: fifteen pending and all sixteen completed outcomes counted, two
// active attempts shed after the newest two, six criteria and six
// constraints dropped from the directive tails — and the header prose never
// reached its fit rungs because the tails were enough.
func TestProjectCountsElidedWork(t *testing.T) {
	proj := Project(maxedPlan(32))
	require.NotNil(t, proj.Elided)
	assert.Equal(t, 15, proj.Elided.Pending)
	assert.Equal(t, 16, proj.Elided.Completed)
	assert.Equal(t, 2, proj.Elided.Attempts)
	assert.Equal(t, 6, proj.Elided.SuccessCriteria)
	assert.Equal(t, 6, proj.Elided.Constraints)
	assert.False(t, proj.Elided.WorkingContext, "directive tails fit the budget first")
	assert.False(t, proj.Elided.Approach)
	assert.False(t, proj.Elided.Goal)
}
