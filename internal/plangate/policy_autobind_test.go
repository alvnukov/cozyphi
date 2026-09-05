package plangate_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The auto-bind contract: a plan_step that names no step, or a step that is
// no longer startable, is bookkeeping, not intent. When exactly one active
// step could take the call the gate binds it and returns the verdict an
// explicit plan_step would have produced; anything less unique keeps the
// miss. The tool-rank miss — the model named a live step the tool does not
// fit — never rebinds: switching a step the model named is not bookkeeping.

func autobindPolicy(t *testing.T) *plangate.Policy {
	t.Helper()
	policy, err := plangate.Compile(plangate.DefaultDefaults())
	require.NoError(t, err)
	return policy
}

func TestAutoBindOmittedStepBindsUniqueCandidate(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanCompleted, Type: session.StepExplore},
		{ID: "wire", Content: "change", Status: session.PlanPending, Type: session.StepEdit},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "write"})

	assert.False(t, v.Miss, "a unique candidate binds instead of missing")
	assert.False(t, v.Deny)
	assert.Equal(t, "wire", v.StepID)
	assert.True(t, v.StartPending, "a pending candidate still auto-starts")
	assert.Contains(t, v.Note, `auto-bound to "wire"`)
	assert.Nil(t, v.JIT)
}

// The bound verdict must be the explicit one, modulo the note: the executor
// applies auto-start, JIT handoff and evidence recording off the same fields.
func TestAutoBindMatchesExplicitVerdict(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		{ID: "wire", Content: "change", Status: session.PlanPending, Type: session.StepEdit},
	}}

	bound := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "write"})
	explicit := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "write", Step: plangate.StepRef{ID: "wire"},
	})

	assert.Equal(t, explicit.StepID, bound.StepID)
	assert.Equal(t, explicit.StartPending, bound.StartPending)
	assert.Equal(t, explicit.JIT, bound.JIT)
	assert.NotEqual(t, explicit.Note, bound.Note, "only the note explains the binding")
}

func TestAutoBindInvalidStepIDBindsUniqueCandidate(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "read", Step: plangate.StepRef{ID: "ghost"},
	})

	require.False(t, v.Miss, "one compatible active step: the wrong id binds to it")
	assert.Equal(t, "survey", v.StepID)
	assert.False(t, v.StartPending)
	assert.Contains(t, v.Note, `"ghost"`)
}

func TestAutoBindCompletedStepBindsUniqueCandidate(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "first", Content: "look", Status: session.PlanCompleted, Type: session.StepExplore},
		{ID: "second", Content: "look again", Status: session.PlanInProgress, Type: session.StepExplore},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "read", Step: plangate.StepRef{ID: "first"},
	})

	require.False(t, v.Miss, "the completed reference falls back to the one live step")
	assert.Equal(t, "second", v.StepID)
	assert.Contains(t, v.Note, `"first"`)
}

func TestAutoBindBlockedStepFallsBackToStartableCandidate(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "stalled", Content: "run", Status: session.PlanBlocked, Type: session.StepRun},
		{ID: "ship", Content: "run again", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "bash", Step: plangate.StepRef{ID: "stalled"},
	})

	require.False(t, v.Miss, "a blocked step is not startable; the unique startable one binds")
	assert.Equal(t, "ship", v.StepID)
	assert.True(t, v.StartPending)
}

func TestAutoBindNumericOutOfRangeKeepsDeprecatedNote(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		{ID: "wire", Content: "change", Status: session.PlanPending, Type: session.StepEdit},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "write", Step: plangate.StepRef{Ordinal: 9},
	})

	require.False(t, v.Miss, "an out-of-range number is corrected when the candidate is unique")
	assert.Equal(t, "wire", v.StepID)
	assert.Contains(t, v.Note, "deprecated", "the numeric input still gets its diagnosis")
	assert.Contains(t, v.Note, `auto-bound to "wire"`)
}

// A numeric reference that resolves keeps today's behavior: it passes with
// the deprecation note and is never re-pointed at a different step.
func TestAutoBindNumericResolvingStepIsNotRebound(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		{ID: "wire", Content: "change", Status: session.PlanPending, Type: session.StepEdit},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "read", Step: plangate.StepRef{Ordinal: 1},
	})

	require.False(t, v.Miss)
	assert.Equal(t, "survey", v.StepID)
	assert.Contains(t, v.Note, "deprecated")
	assert.NotContains(t, v.Note, "auto-bound")
}

func TestAutoBindAmbiguousListsCandidates(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "wire-a", Content: "change", Status: session.PlanInProgress, Type: session.StepEdit},
		{ID: "wire-b", Content: "change more", Status: session.PlanPending, Type: session.StepEdit},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "write"})

	require.True(t, v.Miss, "several candidates: the gate does not guess")
	assert.True(t, v.Deny)
	assert.Contains(t, v.Reason, "compatible steps")
	assert.Contains(t, v.Reason, "wire-a (edit, in_progress)")
	assert.Contains(t, v.Reason, "wire-b (edit, pending)")
	assert.NotEmpty(t, v.Hint)

	hinted := policy.Check(plangate.PhaseHint, plan, plangate.ToolCall{Name: "write"})
	assert.True(t, hinted.Miss)
	assert.False(t, hinted.Deny, "the hint phase keeps guiding, not blocking")
}

func TestAutoBindAmbiguousCandidateListIsBounded(t *testing.T) {
	policy := autobindPolicy(t)
	items := make([]session.PlanItem, 10)
	for i := range items {
		items[i] = session.PlanItem{
			ID:      fmt.Sprintf("step-%02d", i+1),
			Content: "look",
			Status:  session.PlanPending,
			Type:    session.StepExplore,
		}
	}
	plan := session.Plan{Approved: true, Items: items}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "read"})

	require.True(t, v.Miss)
	assert.Contains(t, v.Reason, "step-08 (explore, pending)")
	assert.Contains(t, v.Reason, "+2 more")
	assert.NotContains(t, v.Reason, "step-09", "the list stops at eight entries")
}

func TestAutoBindNoCandidatesKeepsTodaysMiss(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "bash"})

	require.True(t, v.Miss, "nothing could take the call: the miss stands")
	assert.Contains(t, v.Reason, "not a valid step")
	assert.NotContains(t, v.Reason, "compatible steps")
}

func TestAutoBindNamedCompletedStepWithoutCandidatesKeepsTodaysMiss(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "first", Content: "look", Status: session.PlanCompleted, Type: session.StepExplore},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "read", Step: plangate.StepRef{ID: "first"},
	})

	require.True(t, v.Miss)
	assert.Contains(t, v.Reason, "is completed, not an active step")
}

// The safety edge: a live step the named tool does not fit is never silently
// re-pointed at a different one — the model meant that step, so it gets the
// type miss, not a hop.
func TestAutoBindToolRankMissIsNotRebound(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "survey", Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "bash", Step: plangate.StepRef{ID: "survey"},
	})

	require.True(t, v.Miss, "the named step is live; the gate does not switch steps for it")
	assert.Contains(t, v.Reason, `tool "bash" is not allowed on a explore step`)
}

func TestAutoBindUnknownTypeStepIsNotRebound(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "weird", Content: "?", Status: session.PlanInProgress, Type: "custom"},
		{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{
		Name: "bash", Step: plangate.StepRef{ID: "weird"},
	})

	require.True(t, v.Miss)
	assert.Contains(t, v.Reason, "unknown step type")
}

// Legacy plans whose steps carry no ids have nothing bindable: a positional
// number is the only reference they support, and it stays authoritative.
func TestAutoBindLegacyPlanWithoutIDsKeepsTodaysMiss(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{Content: "look", Status: session.PlanInProgress, Type: session.StepExplore},
		{Content: "run", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "bash"})

	require.True(t, v.Miss, "steps without ids cannot be bound")
	assert.Contains(t, v.Reason, "not a valid step")
}

// A unique just-in-time candidate binds and still raises its demand: the
// binding resolves the step, the user grant clears it — one never stands in
// for the other.
func TestAutoBindUniqueJITCandidateStillRaisesDemand(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "ship", Content: "push the tag", Status: session.PlanPending, Type: session.StepRun, JIT: true},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "bash"})

	require.False(t, v.Miss, "the binding is not a miss")
	require.NotNil(t, v.JIT, "auto-binding never substitutes for the JIT grant")
	assert.Equal(t, "ship", v.JIT.StepID)
	assert.Equal(t, "push the tag", v.JIT.Action)
}

func TestAutoBindUnapprovedPlanStaysDenied(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: false, Items: []session.PlanItem{
		{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "bash"})

	require.True(t, v.Miss)
	assert.True(t, v.Deny)
	assert.Contains(t, v.Reason, plangate.ReasonPlanNotApproved)
}

// Exempt tools never require plan_step, so they never auto-bind either: a
// binding on them is voluntary and stays the model's choice.
func TestAutoBindExemptToolStaysUnbound(t *testing.T) {
	policy := autobindPolicy(t)
	plan := session.Plan{Approved: true, Items: []session.PlanItem{
		{ID: "ship", Content: "run", Status: session.PlanPending, Type: session.StepRun},
	}}

	v := policy.Check(plangate.PhaseDeny, plan, plangate.ToolCall{Name: "task"})

	assert.False(t, v.Miss)
	assert.Empty(t, v.StepID, "exemption lifts the requirement, never invents a binding")
	assert.False(t, v.StartPending)
}
